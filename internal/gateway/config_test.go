package gateway

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindConfig_ExplicitPath(t *testing.T) {
	t.Parallel()

	// Create a temp config file
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "custom.yaml")
	if err := os.WriteFile(cfgPath, []byte("transport: stdio\nservers: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := FindConfig(cfgPath)
	if err != nil {
		t.Fatalf("FindConfig(%q) error: %v", cfgPath, err)
	}
	if got != cfgPath {
		t.Errorf("FindConfig(%q) = %q, want %q", cfgPath, got, cfgPath)
	}
}

func TestFindConfig_ExplicitPathNotFound(t *testing.T) {
	t.Parallel()

	_, err := FindConfig("/nonexistent/path/config.yaml")
	if err == nil {
		t.Fatal("expected error for nonexistent explicit path")
	}
	if !strings.Contains(err.Error(), "config file not found") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestFindConfig_CurrentDirectory(t *testing.T) {
	// Change to a temp dir with a config.yaml
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("transport: stdio\nservers: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(origDir) })

	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}

	got, err := FindConfig("")
	if err != nil {
		t.Fatalf("FindConfig(\"\") error: %v", err)
	}
	if got != "config.yaml" {
		t.Errorf("FindConfig(\"\") = %q, want %q", got, "config.yaml")
	}
}

func TestFindConfig_UserConfigDir(t *testing.T) {
	// Change to a temp dir without config.yaml so it falls through
	emptyDir := t.TempDir()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(origDir) })
	if err := os.Chdir(emptyDir); err != nil {
		t.Fatal(err)
	}

	// Create config in user config dir
	userDir, err := os.UserConfigDir()
	if err != nil {
		t.Skip("cannot determine user config dir")
	}

	muxcpDir := filepath.Join(userDir, "muxcp")
	if err := os.MkdirAll(muxcpDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(muxcpDir, "config.yaml")

	// Only create if it doesn't already exist (avoid clobbering real config)
	if _, err := os.Stat(cfgPath); err != nil {
		if err := os.WriteFile(cfgPath, []byte("transport: stdio\nservers: []\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_ = os.Remove(cfgPath)
			_ = os.Remove(muxcpDir)
		})
	}

	got, err := FindConfig("")
	if err != nil {
		t.Fatalf("FindConfig(\"\") error: %v", err)
	}
	if got != cfgPath {
		t.Errorf("FindConfig(\"\") = %q, want %q", got, cfgPath)
	}
}

func TestFindConfig_NoneFound(t *testing.T) {
	// Change to a temp dir with no config anywhere
	emptyDir := t.TempDir()
	origDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(origDir) })
	if err := os.Chdir(emptyDir); err != nil {
		t.Fatal(err)
	}

	// Temporarily ensure user config dir doesn't have muxcp config
	userDir, _ := os.UserConfigDir()
	userCfg := filepath.Join(userDir, "muxcp", "config.yaml")
	if _, err := os.Stat(userCfg); err == nil {
		// File exists — skip this test to avoid messing with real config
		t.Skip("user config exists, skipping none-found test")
	}

	_, err = FindConfig("")
	if err == nil {
		t.Fatal("expected error when no config found")
	}
	if !strings.Contains(err.Error(), "no config file found") {
		t.Errorf("unexpected error: %v", err)
	}
	// Verify the error contains absolute paths
	if !strings.Contains(err.Error(), emptyDir) {
		t.Errorf("error should contain absolute paths, got: %v", err)
	}
}

func TestSystemConfigPath(t *testing.T) {
	t.Parallel()

	path := systemConfigPath()
	if path == "" {
		t.Fatal("systemConfigPath() returned empty string")
	}
	if !strings.HasSuffix(path, "config.yaml") {
		t.Errorf("systemConfigPath() = %q, should end with config.yaml", path)
	}
	if !strings.Contains(path, "muxcp") {
		t.Errorf("systemConfigPath() = %q, should contain muxcp", path)
	}
}

func TestIsContainerRuntime(t *testing.T) {
	t.Parallel()

	tests := []struct {
		command string
		want    bool
	}{
		{"docker", true},
		{"podman", true},
		{"nerdctl", true},
		{"finch", true},
		{"/usr/bin/docker", true},
		{"/usr/local/bin/podman", true},
		{"npx", false},
		{"uvx", false},
		{"node", false},
		{"/usr/bin/python3", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			t.Parallel()
			got := isContainerRuntime(tt.command)
			if got != tt.want {
				t.Errorf("isContainerRuntime(%q) = %v, want %v", tt.command, got, tt.want)
			}
		})
	}
}

func TestResolveRuntimeArgs(t *testing.T) {
	t.Parallel()

	t.Run("basic", func(t *testing.T) {
		t.Parallel()
		cfg := &ServerInstanceConfig{
			Image: "grafana/mcp-grafana",
			Args:  []string{"-t", "stdio"},
			Env:   map[string]string{"GRAFANA_URL": "https://example.com"},
		}
		resolveContainerArgs(cfg)

		// Check base args
		if cfg.Args[0] != "run" || cfg.Args[1] != "--rm" || cfg.Args[2] != "-i" {
			t.Errorf("expected run --rm -i prefix, got %v", cfg.Args[:3])
		}

		// Check -e flag is present
		foundEnv := false
		for i, arg := range cfg.Args {
			if arg == "-e" && i+1 < len(cfg.Args) && cfg.Args[i+1] == "GRAFANA_URL=https://example.com" {
				foundEnv = true
			}
		}
		if !foundEnv {
			t.Errorf("expected -e GRAFANA_URL=https://example.com in args: %v", cfg.Args)
		}

		// Check image is present
		found := false
		for _, arg := range cfg.Args {
			if arg == "grafana/mcp-grafana" {
				found = true
			}
		}
		if !found {
			t.Errorf("expected image in args: %v", cfg.Args)
		}

		// Check trailing args
		last2 := cfg.Args[len(cfg.Args)-2:]
		if last2[0] != "-t" || last2[1] != "stdio" {
			t.Errorf("expected trailing args [-t stdio], got %v", last2)
		}

		// Env should be nil after resolve
		if cfg.Env != nil {
			t.Errorf("expected Env to be nil, got %v", cfg.Env)
		}
	})

	t.Run("no env", func(t *testing.T) {
		t.Parallel()
		cfg := &ServerInstanceConfig{
			Image: "my-image",
			Args:  []string{"--flag"},
		}
		resolveContainerArgs(cfg)

		expected := []string{"run", "--rm", "-i", "my-image", "--flag"}
		if len(cfg.Args) != len(expected) {
			t.Fatalf("args length = %d, want %d: %v", len(cfg.Args), len(expected), cfg.Args)
		}
		for i := range expected {
			if cfg.Args[i] != expected[i] {
				t.Errorf("args[%d] = %q, want %q", i, cfg.Args[i], expected[i])
			}
		}
	})

	t.Run("no args", func(t *testing.T) {
		t.Parallel()
		cfg := &ServerInstanceConfig{
			Image: "my-image",
		}
		resolveContainerArgs(cfg)

		expected := []string{"run", "--rm", "-i", "my-image"}
		if len(cfg.Args) != len(expected) {
			t.Fatalf("args length = %d, want %d: %v", len(cfg.Args), len(expected), cfg.Args)
		}
	})

	t.Run("with volumes", func(t *testing.T) {
		t.Parallel()
		cfg := &ServerInstanceConfig{
			Image:   "my-image",
			Args:    []string{"--flag"},
			Volumes: []string{"/host/path:/container/path", "/data:/data:ro"},
		}
		resolveContainerArgs(cfg)

		// Check -v flags
		volCount := 0
		for i, arg := range cfg.Args {
			if arg == "-v" && i+1 < len(cfg.Args) {
				volCount++
			}
		}
		if volCount != 2 {
			t.Errorf("expected 2 volume mounts, found %d in args: %v", volCount, cfg.Args)
		}

		// Volumes should be nil after resolve
		if cfg.Volumes != nil {
			t.Errorf("expected Volumes to be nil, got %v", cfg.Volumes)
		}
	})

	t.Run("with runtime_args", func(t *testing.T) {
		t.Parallel()
		cfg := &ServerInstanceConfig{
			Image:       "my-image",
			Args:        []string{"serve"},
			RuntimeArgs: []string{"--network", "host", "--privileged"},
		}
		resolveContainerArgs(cfg)

		// runtime_args should appear before the image
		imageIdx := -1
		for i, arg := range cfg.Args {
			if arg == "my-image" {
				imageIdx = i
				break
			}
		}
		if imageIdx < 0 {
			t.Fatalf("image not found in args: %v", cfg.Args)
		}

		// --network should be before the image
		networkIdx := -1
		for i, arg := range cfg.Args {
			if arg == "--network" {
				networkIdx = i
				break
			}
		}
		if networkIdx < 0 || networkIdx >= imageIdx {
			t.Errorf("--network should appear before image, args: %v", cfg.Args)
		}

		// RuntimeArgs should be nil after resolve
		if cfg.RuntimeArgs != nil {
			t.Errorf("expected RuntimeArgs to be nil, got %v", cfg.RuntimeArgs)
		}

		// trailing args should be after image
		lastArg := cfg.Args[len(cfg.Args)-1]
		if lastArg != "serve" {
			t.Errorf("last arg = %q, want \"serve\"", lastArg)
		}
	})

	t.Run("all container options", func(t *testing.T) {
		t.Parallel()
		cfg := &ServerInstanceConfig{
			Image:       "grafana/mcp-grafana",
			Args:        []string{"-t", "stdio"},
			Env:         map[string]string{"KEY": "val"},
			Volumes:     []string{"/a:/b"},
			RuntimeArgs: []string{"--network", "host"},
		}
		resolveContainerArgs(cfg)

		// Verify order: run --rm -i ... -e ... -v ... --network host <image> -t stdio
		if cfg.Args[0] != "run" || cfg.Args[1] != "--rm" || cfg.Args[2] != "-i" {
			t.Errorf("expected run --rm -i prefix, got %v", cfg.Args[:3])
		}

		// Image should be present
		imageIdx := -1
		for i, arg := range cfg.Args {
			if arg == "grafana/mcp-grafana" {
				imageIdx = i
				break
			}
		}
		if imageIdx < 0 {
			t.Fatalf("image not found in args: %v", cfg.Args)
		}

		// Trailing args after image
		remaining := cfg.Args[imageIdx+1:]
		if len(remaining) != 2 || remaining[0] != "-t" || remaining[1] != "stdio" {
			t.Errorf("expected [-t stdio] after image, got %v", remaining)
		}
	})
}

func writeTestConfig(t *testing.T, content string) string {
	t.Helper()
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return cfgPath
}

func TestLoadConfig_Defaults(t *testing.T) {
	t.Parallel()

	cfgPath := writeTestConfig(t, `
servers:
  - name: test
    transport: stdio
    command: echo
`)
	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}

	if cfg.Listen != ":8080" {
		t.Errorf("Listen = %q, want \":8080\"", cfg.Listen)
	}
	if cfg.Transport != TransportSSE {
		t.Errorf("Transport = %q, want %q", cfg.Transport, TransportSSE)
	}
	if cfg.BaseURL != "http://localhost:8080" {
		t.Errorf("BaseURL = %q, want \"http://localhost:8080\"", cfg.BaseURL)
	}
}

func TestLoadConfig_CustomValues(t *testing.T) {
	t.Parallel()

	cfgPath := writeTestConfig(t, `
listen: "0.0.0.0:9090"
transport: stdio
base_url: "https://mcp.example.com"
servers:
  - name: test
    transport: stdio
    command: echo
`)
	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}

	if cfg.Listen != "0.0.0.0:9090" {
		t.Errorf("Listen = %q, want \"0.0.0.0:9090\"", cfg.Listen)
	}
	if cfg.Transport != TransportStdio {
		t.Errorf("Transport = %q, want %q", cfg.Transport, TransportStdio)
	}
	if cfg.BaseURL != "https://mcp.example.com" {
		t.Errorf("BaseURL = %q, want \"https://mcp.example.com\"", cfg.BaseURL)
	}
}

func TestLoadConfig_BaseURLDefaultWithHost(t *testing.T) {
	t.Parallel()

	cfgPath := writeTestConfig(t, `
listen: "10.0.0.1:3000"
servers:
  - name: test
    transport: stdio
    command: echo
`)
	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}

	if cfg.BaseURL != "http://10.0.0.1:3000" {
		t.Errorf("BaseURL = %q, want \"http://10.0.0.1:3000\"", cfg.BaseURL)
	}
}

func TestLoadConfig_EnvExpansion(t *testing.T) {
	t.Setenv("MUXCP_TEST_TOKEN", "secret123")

	cfgPath := writeTestConfig(t, `
servers:
  - name: remote
    transport: sse
    url: "https://mcp.example.com/sse"
    headers:
      Authorization: "Bearer ${MUXCP_TEST_TOKEN}"
`)
	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}

	if cfg.Servers[0].Headers["Authorization"] != "Bearer secret123" {
		t.Errorf("header = %q, want \"Bearer secret123\"", cfg.Servers[0].Headers["Authorization"])
	}
}

func TestLoadConfig_ContainerRuntime(t *testing.T) {
	t.Parallel()

	cfgPath := writeTestConfig(t, `
transport: stdio
servers:
  - name: grafana
    transport: stdio
    command: docker
    image: grafana/mcp-grafana
    args: ["-t", "stdio"]
    env:
      GRAFANA_URL: "https://grafana.example.com"
`)
	cfg, err := LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("LoadConfig error: %v", err)
	}

	s := cfg.Servers[0]
	// Args should have been resolved
	if s.Args[0] != "run" {
		t.Errorf("first arg = %q, want \"run\"", s.Args[0])
	}
	// Env should be nil (moved to -e flags)
	if s.Env != nil {
		t.Errorf("Env should be nil after container resolution, got %v", s.Env)
	}
	// Image should still be set
	if s.Image != "grafana/mcp-grafana" {
		t.Errorf("Image = %q, want \"grafana/mcp-grafana\"", s.Image)
	}
}

func TestLoadConfig_FileNotFound(t *testing.T) {
	t.Parallel()

	_, err := LoadConfig("/nonexistent/config.yaml")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestLoadConfig_InvalidYAML(t *testing.T) {
	t.Parallel()

	cfgPath := writeTestConfig(t, `{{{invalid yaml`)
	_, err := LoadConfig(cfgPath)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestValidateConfig_MissingName(t *testing.T) {
	t.Parallel()

	cfg := &GatewayConfig{
		Servers: []ServerInstanceConfig{
			{Transport: TransportStdio, Command: "echo"},
		},
	}
	err := validateConfig(cfg)
	if err == nil || !strings.Contains(err.Error(), "name is required") {
		t.Errorf("expected 'name is required' error, got %v", err)
	}
}

func TestValidateConfig_DuplicateName(t *testing.T) {
	t.Parallel()

	cfg := &GatewayConfig{
		Servers: []ServerInstanceConfig{
			{Name: "dup", Transport: TransportStdio, Command: "echo"},
			{Name: "dup", Transport: TransportStdio, Command: "echo"},
		},
	}
	err := validateConfig(cfg)
	if err == nil || !strings.Contains(err.Error(), "duplicate name") {
		t.Errorf("expected 'duplicate name' error, got %v", err)
	}
}

func TestValidateConfig_StdioMissingCommand(t *testing.T) {
	t.Parallel()

	cfg := &GatewayConfig{
		Servers: []ServerInstanceConfig{
			{Name: "test", Transport: TransportStdio},
		},
	}
	err := validateConfig(cfg)
	if err == nil || !strings.Contains(err.Error(), "command is required") {
		t.Errorf("expected 'command is required' error, got %v", err)
	}
}

func TestValidateConfig_ContainerMissingImage(t *testing.T) {
	t.Parallel()

	cfg := &GatewayConfig{
		Servers: []ServerInstanceConfig{
			{Name: "test", Transport: TransportStdio, Command: "docker"},
		},
	}
	err := validateConfig(cfg)
	if err == nil || !strings.Contains(err.Error(), "image is required") {
		t.Errorf("expected 'image is required' error, got %v", err)
	}
}

func TestValidateConfig_SSEMissingURL(t *testing.T) {
	t.Parallel()

	cfg := &GatewayConfig{
		Servers: []ServerInstanceConfig{
			{Name: "test", Transport: TransportSSE},
		},
	}
	err := validateConfig(cfg)
	if err == nil || !strings.Contains(err.Error(), "url is required") {
		t.Errorf("expected 'url is required' error, got %v", err)
	}
}

func TestValidateConfig_StreamableHTTPMissingURL(t *testing.T) {
	t.Parallel()

	cfg := &GatewayConfig{
		Servers: []ServerInstanceConfig{
			{Name: "test", Transport: TransportStreamableHTTP},
		},
	}
	err := validateConfig(cfg)
	if err == nil || !strings.Contains(err.Error(), "url is required") {
		t.Errorf("expected 'url is required' error, got %v", err)
	}
}

func TestValidateConfig_UnknownTransport(t *testing.T) {
	t.Parallel()

	cfg := &GatewayConfig{
		Servers: []ServerInstanceConfig{
			{Name: "test", Transport: "grpc"},
		},
	}
	err := validateConfig(cfg)
	if err == nil || !strings.Contains(err.Error(), "unknown transport") {
		t.Errorf("expected 'unknown transport' error, got %v", err)
	}
}

func TestValidateConfig_ValidSSE(t *testing.T) {
	t.Parallel()

	cfg := &GatewayConfig{
		Servers: []ServerInstanceConfig{
			{Name: "remote", Transport: TransportSSE, URL: "https://example.com/sse"},
		},
	}
	if err := validateConfig(cfg); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateConfig_ValidStreamableHTTP(t *testing.T) {
	t.Parallel()

	cfg := &GatewayConfig{
		Servers: []ServerInstanceConfig{
			{Name: "remote", Transport: TransportStreamableHTTP, URL: "https://example.com/mcp"},
		},
	}
	if err := validateConfig(cfg); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateConfig_ValidStdio(t *testing.T) {
	t.Parallel()

	cfg := &GatewayConfig{
		Servers: []ServerInstanceConfig{
			{Name: "local", Transport: TransportStdio, Command: "echo"},
		},
	}
	if err := validateConfig(cfg); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateConfig_EmptyServers(t *testing.T) {
	t.Parallel()

	cfg := &GatewayConfig{}
	if err := validateConfig(cfg); err != nil {
		t.Errorf("unexpected error for empty servers: %v", err)
	}
}

func TestValidateConfig_MultipleValidServers(t *testing.T) {
	t.Parallel()

	cfg := &GatewayConfig{
		Servers: []ServerInstanceConfig{
			{Name: "stdio1", Transport: TransportStdio, Command: "echo"},
			{Name: "sse1", Transport: TransportSSE, URL: "https://example.com/sse"},
			{Name: "docker1", Transport: TransportStdio, Command: "docker", Image: "my-image"},
		},
	}
	if err := validateConfig(cfg); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
