package gateway

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const namespaceSep = "__"

// Gateway aggregates multiple MCP backends behind a single MCP server.
type Gateway struct {
	config   *GatewayConfig
	backends []*Backend
	// Maps namespaced tool name → backend
	toolRoute map[string]*Backend
	// Maps namespaced tool name → original tool name on the backend
	toolOrigName map[string]string
	mcpServer    *server.MCPServer
	mu           sync.RWMutex
}

// NewGateway creates a new gateway with the given configuration.
func NewGateway(cfg *GatewayConfig) *Gateway {
	return &Gateway{
		config:       cfg,
		toolRoute:    make(map[string]*Backend),
		toolOrigName: make(map[string]string),
	}
}

// Start connects to all backends, builds the aggregated tool set, and starts the MCP server.
func (g *Gateway) Start(ctx context.Context) error {
	// Connect to all backends
	for _, scfg := range g.config.Servers {
		b, err := NewBackend(ctx, scfg)
		if err != nil {
			// Close already-connected backends on failure
			g.closeBackends()
			return fmt.Errorf("connecting backend %q: %w", scfg.Name, err)
		}
		g.backends = append(g.backends, b)
	}

	// Build the MCP server with aggregated tools
	g.mcpServer = server.NewMCPServer(
		"muxcp",
		"1.0.0",
		server.WithToolCapabilities(false),
		server.WithRecovery(),
	)

	g.registerTools()

	// Start the appropriate transport
	switch g.config.Transport {
	case TransportStdio:
		return g.serveStdio()
	case TransportSSE:
		return g.serveSSE()
	case TransportStreamableHTTP:
		return g.serveStreamableHTTP()
	default:
		return fmt.Errorf("unsupported gateway transport: %s", g.config.Transport)
	}
}

func (g *Gateway) registerTools() {
	var tools []server.ServerTool

	for _, b := range g.backends {
		b.mu.RLock()
		for i := range b.Tools {
			t := &b.Tools[i]
			nsName := b.Name + namespaceSep + t.Name
			namespacedTool := cloneToolWithName(*t, nsName)

			g.mu.Lock()
			g.toolRoute[nsName] = b
			g.toolOrigName[nsName] = t.Name
			g.mu.Unlock()

			tools = append(tools, server.ServerTool{
				Tool:    namespacedTool,
				Handler: g.makeHandler(nsName),
			})
		}
		b.mu.RUnlock()
	}

	if len(tools) > 0 {
		g.mcpServer.AddTools(tools...)
	}

	slog.Info("gateway tools registered", "count", len(tools))
}

func (g *Gateway) makeHandler(nsName string) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		g.mu.RLock()
		backend, ok := g.toolRoute[nsName]
		origName := g.toolOrigName[nsName]
		g.mu.RUnlock()

		if !ok {
			return mcp.NewToolResultError("unknown tool: " + nsName), nil
		}

		args := req.GetArguments()
		slog.Info("routing tool call", "tool", nsName, "backend", backend.Name, "origTool", origName)

		result, err := backend.CallTool(ctx, origName, args)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("backend %q error: %v", backend.Name, err)), nil
		}
		return result, nil
	}
}

func (g *Gateway) serveStdio() error {
	slog.Info("gateway serving on stdio")
	return server.ServeStdio(g.mcpServer)
}

func (g *Gateway) serveSSE() error {
	sseServer := server.NewSSEServer(g.mcpServer,
		server.WithBaseURL(g.config.BaseURL),
		server.WithKeepAlive(true),
	)
	slog.Info("gateway listening", "transport", "sse", "address", g.config.Listen)
	return sseServer.Start(g.config.Listen)
}

func (g *Gateway) serveStreamableHTTP() error {
	httpServer := server.NewStreamableHTTPServer(g.mcpServer)
	slog.Info("gateway listening", "transport", "streamable-http", "address", g.config.Listen)
	return httpServer.Start(g.config.Listen)
}

func (g *Gateway) closeBackends() {
	for _, b := range g.backends {
		if err := b.Close(); err != nil {
			slog.Error("closing backend", "name", b.Name, "error", err)
		}
	}
}

// Shutdown gracefully stops the gateway.
func (g *Gateway) Shutdown() {
	g.closeBackends()
}

// cloneToolWithName creates a copy of a tool with a new name, preserving description and schema.
func cloneToolWithName(t mcp.Tool, newName string) mcp.Tool {
	// Build a description that includes the original backend context
	parts := strings.SplitN(newName, namespaceSep, 2)
	desc := t.Description
	if len(parts) == 2 {
		desc = fmt.Sprintf("[%s] %s", parts[0], t.Description)
	}

	clone := mcp.Tool{
		Name:        newName,
		Description: desc,
		InputSchema: t.InputSchema,
	}
	clone.Annotations = t.Annotations
	return clone
}
