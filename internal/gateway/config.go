package gateway

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// TransportType defines the transport protocol for MCP communication.
type TransportType string

// Supported transport types.
const (
	TransportStdio          TransportType = "stdio"
	TransportSSE            TransportType = "sse"
	TransportStreamableHTTP TransportType = "streamable-http"
)

// ServerInstanceConfig defines the configuration for a single upstream MCP server.
type ServerInstanceConfig struct {
	Name      string            `yaml:"name"`
	Transport TransportType     `yaml:"transport"`
	Command   string            `yaml:"command,omitempty"` // for stdio
	Args      []string          `yaml:"args,omitempty"`    // for stdio
	Image     string            `yaml:"image,omitempty"`   // container image (for docker/podman)
	URL       string            `yaml:"url,omitempty"`     // for sse / streamable-http
	Headers   map[string]string `yaml:"headers,omitempty"` // for remote transports
	Env       map[string]string `yaml:"env,omitempty"`     // for stdio (extra env vars)
}

// GatewayConfig defines the top-level gateway configuration.
type GatewayConfig struct {
	Listen    string                 `yaml:"listen"`
	BaseURL   string                 `yaml:"base_url"`  // externally-reachable URL for SSE message endpoint
	Transport TransportType          `yaml:"transport"` // what transport to expose: stdio, sse, or streamable-http
	Servers   []ServerInstanceConfig `yaml:"servers"`
}

// LoadConfig reads and parses the gateway configuration from a YAML file.
func LoadConfig(path string) (*GatewayConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	// Expand environment variables in the config
	expanded := os.ExpandEnv(string(data))

	var cfg GatewayConfig
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	if cfg.Listen == "" {
		cfg.Listen = ":8080"
	}
	if cfg.Transport == "" {
		cfg.Transport = TransportSSE
	}
	if cfg.BaseURL == "" {
		host := cfg.Listen
		if host[0] == ':' {
			host = "localhost" + host
		}
		cfg.BaseURL = "http://" + host
	}

	if err := validateConfig(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// isContainerRuntime returns true if the command is docker, podman, or similar.
func isContainerRuntime(command string) bool {
	base := filepath.Base(command)
	switch base {
	case "docker", "podman", "nerdctl", "finch":
		return true
	}
	return false
}

// resolveContainerArgs builds the full args for a container runtime command.
// It prepends "run --rm -i", adds -e flags for env vars, the image, then the user args.
func resolveContainerArgs(cfg *ServerInstanceConfig) {
	// 3 base args + 2 per env var + 1 image + user args
	args := make([]string, 0, 3+2*len(cfg.Env)+1+len(cfg.Args))
	args = append(args, "run", "--rm", "-i")

	for k, v := range cfg.Env {
		args = append(args, "-e", k+"="+v)
	}

	args = append(args, cfg.Image)
	args = append(args, cfg.Args...)

	cfg.Args = args
	cfg.Env = nil // env is passed via -e, not process env
}

func validateConfig(cfg *GatewayConfig) error {
	names := make(map[string]bool)
	for i := range cfg.Servers {
		s := &cfg.Servers[i]

		if s.Name == "" {
			return fmt.Errorf("server[%d]: name is required", i)
		}
		if names[s.Name] {
			return fmt.Errorf("server[%d]: duplicate name %q", i, s.Name)
		}
		names[s.Name] = true

		switch s.Transport {
		case TransportStdio:
			if s.Command == "" {
				return fmt.Errorf("server %q: command is required for stdio transport", s.Name)
			}
			if isContainerRuntime(s.Command) {
				if s.Image == "" {
					return fmt.Errorf("server %q: image is required when using %s", s.Name, s.Command)
				}
				resolveContainerArgs(s)
			}
		case TransportSSE, TransportStreamableHTTP:
			if s.URL == "" {
				return fmt.Errorf("server %q: url is required for %s transport", s.Name, s.Transport)
			}
		default:
			return fmt.Errorf("server %q: unknown transport %q", s.Name, s.Transport)
		}
	}
	return nil
}
