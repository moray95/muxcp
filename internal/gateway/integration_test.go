package gateway

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// startTestMCPServer starts a simple MCP server using httptest and returns the URL.
func startTestMCPServer(t *testing.T, tools ...server.ServerTool) string {
	t.Helper()

	s := server.NewMCPServer("test-backend", "1.0.0", server.WithToolCapabilities(false))
	if len(tools) > 0 {
		s.AddTools(tools...)
	}

	handler := server.NewStreamableHTTPServer(s)
	ts := httptest.NewServer(handler)
	t.Cleanup(ts.Close)

	return ts.URL
}

func echoTool() server.ServerTool {
	return server.ServerTool{
		Tool: mcp.Tool{
			Name:        "echo",
			Description: "Echoes back the input",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]any{
					"message": map[string]any{
						"type":        "string",
						"description": "The message to echo",
					},
				},
				Required: []string{"message"},
			},
		},
		Handler: func(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			msg, _ := req.GetArguments()["message"].(string)
			return mcp.NewToolResultText("echo: " + msg), nil
		},
	}
}

func greetTool() server.ServerTool {
	return server.ServerTool{
		Tool: mcp.Tool{
			Name:        "greet",
			Description: "Greets the user",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]any{
					"name": map[string]any{"type": "string"},
				},
			},
		},
		Handler: func(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			name, _ := req.GetArguments()["name"].(string)
			return mcp.NewToolResultText("hello " + name), nil
		},
	}
}

func TestNewBackend_StreamableHTTP(t *testing.T) {
	t.Parallel()

	url := startTestMCPServer(t, echoTool(), greetTool())

	ctx := context.Background()
	cfg := ServerInstanceConfig{
		Name:      "test-backend",
		Transport: TransportStreamableHTTP,
		URL:       url,
	}

	b, err := NewBackend(ctx, cfg)
	if err != nil {
		t.Fatalf("NewBackend error: %v", err)
	}
	defer b.Close()

	if b.Name != "test-backend" {
		t.Errorf("Name = %q, want %q", b.Name, "test-backend")
	}
	if len(b.Tools) != 2 {
		t.Errorf("Tools count = %d, want 2", len(b.Tools))
	}
}

func TestBackendCallTool(t *testing.T) {
	t.Parallel()

	url := startTestMCPServer(t, echoTool())

	ctx := context.Background()
	b, err := NewBackend(ctx, ServerInstanceConfig{
		Name:      "test",
		Transport: TransportStreamableHTTP,
		URL:       url,
	})
	if err != nil {
		t.Fatalf("NewBackend error: %v", err)
	}
	defer b.Close()

	result, err := b.CallTool(ctx, "echo", map[string]any{"message": "hello world"})
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result.IsError {
		t.Fatalf("CallTool returned error result")
	}

	if len(result.Content) == 0 {
		t.Fatal("empty result content")
	}
	textContent, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	if textContent.Text != "echo: hello world" {
		t.Errorf("result = %q, want \"echo: hello world\"", textContent.Text)
	}
}

func TestBackendRefreshTools(t *testing.T) {
	t.Parallel()

	url := startTestMCPServer(t, echoTool())

	ctx := context.Background()
	b, err := NewBackend(ctx, ServerInstanceConfig{
		Name:      "test",
		Transport: TransportStreamableHTTP,
		URL:       url,
	})
	if err != nil {
		t.Fatalf("NewBackend error: %v", err)
	}
	defer b.Close()

	if len(b.Tools) != 1 {
		t.Fatalf("initial tools = %d, want 1", len(b.Tools))
	}

	if err := b.refreshTools(ctx); err != nil {
		t.Fatalf("refreshTools error: %v", err)
	}
	if len(b.Tools) != 1 {
		t.Errorf("tools after refresh = %d, want 1", len(b.Tools))
	}
}

func TestBackendClose_WithClient(t *testing.T) {
	t.Parallel()

	url := startTestMCPServer(t, echoTool())

	ctx := context.Background()
	b, err := NewBackend(ctx, ServerInstanceConfig{
		Name:      "test",
		Transport: TransportStreamableHTTP,
		URL:       url,
	})
	if err != nil {
		t.Fatalf("NewBackend error: %v", err)
	}

	if err := b.Close(); err != nil {
		t.Errorf("Close() error: %v", err)
	}
}

func TestNewBackend_UnsupportedTransport(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	_, err := NewBackend(ctx, ServerInstanceConfig{
		Name:      "test",
		Transport: "grpc",
	})
	if err == nil {
		t.Fatal("expected error for unsupported transport")
	}
}

func TestNewBackend_InvalidURL(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	_, err := NewBackend(ctx, ServerInstanceConfig{
		Name:      "test",
		Transport: TransportStreamableHTTP,
		URL:       "http://127.0.0.1:1",
	})
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
}

func TestGatewayStartWithBackends(t *testing.T) {
	t.Parallel()

	url1 := startTestMCPServer(t, echoTool())
	url2 := startTestMCPServer(t, greetTool())

	cfg := &GatewayConfig{
		Listen:    "127.0.0.1:0",
		Transport: TransportStreamableHTTP,
		BaseURL:   "http://localhost:0",
		Servers: []ServerInstanceConfig{
			{Name: "s1", Transport: TransportStreamableHTTP, URL: url1},
			{Name: "s2", Transport: TransportStreamableHTTP, URL: url2},
		},
	}

	gw := NewGateway(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- gw.Start(ctx)
	}()

	time.Sleep(300 * time.Millisecond)

	gw.mu.RLock()
	routeCount := len(gw.toolRoute)
	_, hasS1Echo := gw.toolRoute["s1__echo"]
	_, hasS2Greet := gw.toolRoute["s2__greet"]
	gw.mu.RUnlock()

	if routeCount != 2 {
		t.Errorf("toolRoute has %d entries, want 2", routeCount)
	}
	if !hasS1Echo {
		t.Error("missing s1__echo in toolRoute")
	}
	if !hasS2Greet {
		t.Error("missing s2__greet in toolRoute")
	}

	gw.Shutdown()
	cancel()
}

func TestGatewayStart_BackendFailure(t *testing.T) {
	t.Parallel()

	cfg := &GatewayConfig{
		Transport: TransportStreamableHTTP,
		Listen:    "127.0.0.1:0",
		BaseURL:   "http://localhost:0",
		Servers: []ServerInstanceConfig{
			{Name: "bad", Transport: TransportStreamableHTTP, URL: "http://127.0.0.1:1"},
		},
	}

	gw := NewGateway(cfg)
	err := gw.Start(context.Background())
	if err == nil {
		t.Fatal("expected error when backend fails to connect")
	}
}

func TestGatewayStart_UnsupportedTransport(t *testing.T) {
	t.Parallel()

	url := startTestMCPServer(t, echoTool())

	cfg := &GatewayConfig{
		Transport: "grpc",
		Servers: []ServerInstanceConfig{
			{Name: "s1", Transport: TransportStreamableHTTP, URL: url},
		},
	}

	gw := NewGateway(cfg)
	err := gw.Start(context.Background())
	if err == nil {
		t.Fatal("expected error for unsupported gateway transport")
	}
}

func TestGatewayMakeHandler(t *testing.T) {
	t.Parallel()

	url := startTestMCPServer(t, echoTool())

	ctx := context.Background()
	b, err := NewBackend(ctx, ServerInstanceConfig{
		Name:      "test",
		Transport: TransportStreamableHTTP,
		URL:       url,
	})
	if err != nil {
		t.Fatalf("NewBackend error: %v", err)
	}
	defer b.Close()

	cfg := &GatewayConfig{Transport: TransportStdio}
	gw := NewGateway(cfg)
	gw.backends = []*Backend{b}
	gw.mcpServer = newTestMCPServer()
	gw.registerTools()

	handler := gw.makeHandler("test__echo")
	req := mcp.CallToolRequest{}
	req.Params.Name = "test__echo"
	req.Params.Arguments = map[string]any{"message": "test"}

	result, err := handler(ctx, req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	textContent, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	if textContent.Text != "echo: test" {
		t.Errorf("result = %q, want \"echo: test\"", textContent.Text)
	}
}

func TestGatewayMakeHandler_UnknownTool(t *testing.T) {
	t.Parallel()

	cfg := &GatewayConfig{Transport: TransportStdio}
	gw := NewGateway(cfg)

	handler := gw.makeHandler("nonexistent__tool")
	req := mcp.CallToolRequest{}
	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error result for unknown tool")
	}
}

func TestGatewayMakeHandler_BackendError(t *testing.T) {
	t.Parallel()

	url := startTestMCPServer(t, echoTool())

	ctx := context.Background()
	b, err := NewBackend(ctx, ServerInstanceConfig{
		Name:      "test",
		Transport: TransportStreamableHTTP,
		URL:       url,
	})
	if err != nil {
		t.Fatalf("NewBackend error: %v", err)
	}

	cfg := &GatewayConfig{Transport: TransportStdio}
	gw := NewGateway(cfg)
	gw.backends = []*Backend{b}
	gw.mcpServer = newTestMCPServer()
	gw.registerTools()

	// Close the backend to simulate failure
	b.Close()

	handler := gw.makeHandler("test__echo")
	req := mcp.CallToolRequest{}
	req.Params.Name = "test__echo"
	req.Params.Arguments = map[string]any{"message": "test"}

	result, err := handler(ctx, req)
	if err != nil {
		t.Fatalf("handler should not return error, got: %v", err)
	}
	if !result.IsError {
		t.Error("expected error result when backend is closed")
	}
}

func TestGatewayFullIntegration(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	url := startTestMCPServer(t, echoTool(), greetTool())

	cfgContent := `
transport: streamable-http
listen: "127.0.0.1:0"
servers:
  - name: backend1
    transport: streamable-http
    url: "` + url + `"
`
	cfgPath := filepath.Join(tmp, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}

	gw := NewGateway(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		_ = gw.Start(ctx)
	}()

	time.Sleep(300 * time.Millisecond)

	gw.mu.RLock()
	count := len(gw.toolRoute)
	gw.mu.RUnlock()

	if count != 2 {
		t.Errorf("expected 2 tools registered, got %d", count)
	}

	gw.Shutdown()
	cancel()
}

func TestGatewayCloseBackends_WithError(t *testing.T) {
	t.Parallel()

	url := startTestMCPServer(t, echoTool())

	ctx := context.Background()
	b, err := NewBackend(ctx, ServerInstanceConfig{
		Name:      "test",
		Transport: TransportStreamableHTTP,
		URL:       url,
	})
	if err != nil {
		t.Fatalf("NewBackend error: %v", err)
	}

	cfg := &GatewayConfig{}
	gw := NewGateway(cfg)
	gw.backends = []*Backend{b}

	// Should not panic
	gw.closeBackends()
}
