package gateway

import (
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func TestNewGateway(t *testing.T) {
	t.Parallel()

	cfg := &GatewayConfig{
		Listen:    ":8080",
		Transport: TransportStdio,
	}
	gw := NewGateway(cfg)

	if gw.config != cfg {
		t.Error("config not set")
	}
	if gw.toolRoute == nil {
		t.Error("toolRoute not initialized")
	}
	if gw.toolOrigName == nil {
		t.Error("toolOrigName not initialized")
	}
	if gw.backends != nil {
		t.Error("backends should be nil initially")
	}
}

func TestCloneToolWithName(t *testing.T) {
	t.Parallel()

	t.Run("namespaced name", func(t *testing.T) {
		t.Parallel()
		original := mcp.Tool{
			Name:        "search",
			Description: "Search for items",
			InputSchema: mcp.ToolInputSchema{
				Type: "object",
				Properties: map[string]any{
					"query": map[string]any{"type": "string"},
				},
			},
		}

		clone := cloneToolWithName(original, "backend1__search")

		if clone.Name != "backend1__search" {
			t.Errorf("Name = %q, want %q", clone.Name, "backend1__search")
		}
		if clone.Description != "[backend1] Search for items" {
			t.Errorf("Description = %q, want %q", clone.Description, "[backend1] Search for items")
		}
		if clone.InputSchema.Type != "object" {
			t.Errorf("InputSchema.Type = %q, want \"object\"", clone.InputSchema.Type)
		}
		if _, ok := clone.InputSchema.Properties["query"]; !ok {
			t.Error("InputSchema.Properties should contain 'query'")
		}
	})

	t.Run("no namespace separator", func(t *testing.T) {
		t.Parallel()
		original := mcp.Tool{
			Name:        "search",
			Description: "Search for items",
		}

		clone := cloneToolWithName(original, "search")

		if clone.Name != "search" {
			t.Errorf("Name = %q, want %q", clone.Name, "search")
		}
		if clone.Description != "Search for items" {
			t.Errorf("Description = %q, want %q", clone.Description, "Search for items")
		}
	})

	t.Run("preserves annotations", func(t *testing.T) {
		t.Parallel()
		original := mcp.Tool{
			Name:        "tool1",
			Description: "A tool",
			Annotations: mcp.ToolAnnotation{
				Title:           "My Tool",
				ReadOnlyHint:    boolPtr(true),
				DestructiveHint: boolPtr(false),
			},
		}

		clone := cloneToolWithName(original, "ns__tool1")

		if clone.Annotations.Title != "My Tool" {
			t.Errorf("Annotations.Title = %q, want \"My Tool\"", clone.Annotations.Title)
		}
		if *clone.Annotations.ReadOnlyHint != true {
			t.Error("ReadOnlyHint should be true")
		}
	})

	t.Run("empty description", func(t *testing.T) {
		t.Parallel()
		original := mcp.Tool{
			Name:        "tool",
			Description: "",
		}

		clone := cloneToolWithName(original, "ns__tool")

		if clone.Description != "[ns] " {
			t.Errorf("Description = %q, want \"[ns] \"", clone.Description)
		}
	})
}

func TestRegisterTools(t *testing.T) {
	t.Parallel()

	cfg := &GatewayConfig{Transport: TransportStdio}
	gw := NewGateway(cfg)

	gw.backends = []*Backend{
		{
			Name: "b1",
			Tools: []mcp.Tool{
				{Name: "tool_a", Description: "Tool A"},
				{Name: "tool_b", Description: "Tool B"},
			},
		},
		{
			Name: "b2",
			Tools: []mcp.Tool{
				{Name: "tool_a", Description: "Tool A from b2"},
			},
		},
	}

	gw.mcpServer = newTestMCPServer()
	gw.registerTools()

	if len(gw.toolRoute) != 3 {
		t.Errorf("toolRoute has %d entries, want 3", len(gw.toolRoute))
	}

	tests := []struct {
		nsName   string
		backend  string
		origName string
	}{
		{"b1__tool_a", "b1", "tool_a"},
		{"b1__tool_b", "b1", "tool_b"},
		{"b2__tool_a", "b2", "tool_a"},
	}

	for _, tt := range tests {
		b, ok := gw.toolRoute[tt.nsName]
		if !ok {
			t.Errorf("toolRoute missing %q", tt.nsName)
			continue
		}
		if b.Name != tt.backend {
			t.Errorf("toolRoute[%q].Name = %q, want %q", tt.nsName, b.Name, tt.backend)
		}
		if gw.toolOrigName[tt.nsName] != tt.origName {
			t.Errorf("toolOrigName[%q] = %q, want %q", tt.nsName, gw.toolOrigName[tt.nsName], tt.origName)
		}
	}
}

func TestRegisterTools_NoBackends(t *testing.T) {
	t.Parallel()

	cfg := &GatewayConfig{Transport: TransportStdio}
	gw := NewGateway(cfg)
	gw.mcpServer = newTestMCPServer()
	gw.registerTools()

	if len(gw.toolRoute) != 0 {
		t.Errorf("toolRoute has %d entries, want 0", len(gw.toolRoute))
	}
}

func TestRegisterTools_EmptyBackend(t *testing.T) {
	t.Parallel()

	cfg := &GatewayConfig{Transport: TransportStdio}
	gw := NewGateway(cfg)
	gw.backends = []*Backend{
		{Name: "empty"},
	}
	gw.mcpServer = newTestMCPServer()
	gw.registerTools()

	if len(gw.toolRoute) != 0 {
		t.Errorf("toolRoute has %d entries, want 0", len(gw.toolRoute))
	}
}

func TestShutdown_NoBackends(t *testing.T) {
	t.Parallel()

	cfg := &GatewayConfig{}
	gw := NewGateway(cfg)
	gw.Shutdown()
}

func TestShutdown_WithBackends(t *testing.T) {
	t.Parallel()

	cfg := &GatewayConfig{}
	gw := NewGateway(cfg)
	gw.backends = []*Backend{
		{Name: "b1", Client: nil},
		{Name: "b2", Client: nil},
	}
	gw.Shutdown()
}

func newTestMCPServer() *server.MCPServer {
	return server.NewMCPServer("test", "1.0.0", server.WithToolCapabilities(false))
}

func boolPtr(b bool) *bool {
	return &b
}
