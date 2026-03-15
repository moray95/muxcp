package gateway

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
)

// Backend represents a single upstream MCP server connection.
type Backend struct {
	Name   string
	Config ServerInstanceConfig
	Client *client.Client
	Tools  []mcp.Tool
	mu     sync.RWMutex
}

// NewBackend creates and initializes a connection to an upstream MCP server.
func NewBackend(ctx context.Context, cfg ServerInstanceConfig) (*Backend, error) {
	b := &Backend{
		Name:   cfg.Name,
		Config: cfg,
	}

	if err := b.connect(ctx); err != nil {
		return nil, fmt.Errorf("backend %q: %w", cfg.Name, err)
	}

	return b, nil
}

func (b *Backend) connect(ctx context.Context) error {
	var (
		c   *client.Client
		err error
	)

	switch b.Config.Transport {
	case TransportStdio:
		env := buildEnv(b.Config.Env)
		c, err = client.NewStdioMCPClient(b.Config.Command, env, b.Config.Args...)
		if err != nil {
			return fmt.Errorf("creating stdio client: %w", err)
		}

	case TransportSSE:
		opts := buildSSEOpts(b.Config.Headers)
		c, err = client.NewSSEMCPClient(b.Config.URL, opts...)
		if err != nil {
			return fmt.Errorf("creating SSE client: %w", err)
		}
		if err := c.Start(ctx); err != nil {
			return fmt.Errorf("starting SSE client: %w", err)
		}

	case TransportStreamableHTTP:
		opts := buildStreamableHTTPOpts(b.Config.Headers)
		c, err = client.NewStreamableHttpClient(b.Config.URL, opts...)
		if err != nil {
			return fmt.Errorf("creating streamable HTTP client: %w", err)
		}
		if err := c.Start(ctx); err != nil {
			return fmt.Errorf("starting streamable HTTP client: %w", err)
		}

	default:
		return fmt.Errorf("unsupported transport: %s", b.Config.Transport)
	}

	// Initialize the MCP session
	initReq := mcp.InitializeRequest{}
	initReq.Params.ClientInfo = mcp.Implementation{
		Name:    "mcp-gateway",
		Version: "1.0.0",
	}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION

	_, err = c.Initialize(ctx, initReq)
	if err != nil {
		_ = c.Close()
		return fmt.Errorf("initializing: %w", err)
	}

	b.Client = c

	// Discover tools
	if err := b.refreshTools(ctx); err != nil {
		_ = c.Close()
		return fmt.Errorf("listing tools: %w", err)
	}

	slog.Info("backend connected", "name", b.Name, "tools", len(b.Tools))
	return nil
}

func (b *Backend) refreshTools(ctx context.Context) error {
	result, err := b.Client.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		return err
	}

	b.mu.Lock()
	b.Tools = result.Tools
	b.mu.Unlock()

	return nil
}

// CallTool forwards a tool call to this backend.
func (b *Backend) CallTool(ctx context.Context, name string, args map[string]any) (*mcp.CallToolResult, error) {
	req := mcp.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args
	return b.Client.CallTool(ctx, req)
}

// Close shuts down the backend connection.
func (b *Backend) Close() error {
	if b.Client != nil {
		return b.Client.Close()
	}
	return nil
}

// buildEnv converts a map of env vars to the []string format expected by the stdio client.
// Extra vars are appended to the current process environment.
func buildEnv(extra map[string]string) []string {
	if len(extra) == 0 {
		return nil // inherit parent env
	}
	// Start with empty slice — stdio client inherits parent env when nil,
	// but when we provide a list it replaces the env entirely.
	// So we must include the parent env too.
	env := make([]string, 0, len(extra))
	for k, v := range extra {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	return env
}

func buildSSEOpts(headers map[string]string) []transport.ClientOption {
	var opts []transport.ClientOption
	if len(headers) > 0 {
		opts = append(opts, transport.WithHeaders(headers))
	}
	return opts
}

func buildStreamableHTTPOpts(headers map[string]string) []transport.StreamableHTTPCOption {
	var opts []transport.StreamableHTTPCOption
	if len(headers) > 0 {
		opts = append(opts, transport.WithHTTPHeaders(headers))
	}
	return opts
}
