package gateway

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"gopkg.in/yaml.v3"
)

const configFileName = "config.yaml"

// FindConfig resolves the config file path using the following precedence:
// 1. Explicitly provided path (from -config flag)
// 2. ./config.yaml (current directory)
// 3. User config dir (~/.config/muxcp/config.yaml on Linux/macOS, %AppData%\muxcp on Windows)
// 4. System config dir (/etc/muxcp/config.yaml on Unix, %ProgramData%\muxcp on Windows)
func FindConfig(explicit string) (string, error) {
	if explicit != "" {
		if _, err := os.Stat(explicit); err == nil {
			return explicit, nil
		}
		return "", fmt.Errorf("config file not found: %s", explicit)
	}

	var candidates []string

	candidates = append(candidates, configFileName)

	if userDir, err := os.UserConfigDir(); err == nil {
		candidates = append(candidates, filepath.Join(userDir, "muxcp", configFileName))
	}

	candidates = append(candidates, systemConfigPath())

	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	// Resolve all paths to absolute for a clear error message
	searched := make([]string, 0, len(candidates))
	for _, c := range candidates {
		if abs, err := filepath.Abs(c); err == nil {
			searched = append(searched, abs)
		} else {
			searched = append(searched, c)
		}
	}

	return "", fmt.Errorf("no config file found; searched: %s", strings.Join(searched, ", "))
}

func systemConfigPath() string {
	if runtime.GOOS == "windows" {
		pd := os.Getenv("ProgramData")
		if pd == "" {
			pd = `C:\ProgramData`
		}
		return pd + `\muxcp\` + configFileName
	}
	return "/etc/muxcp/" + configFileName
}

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
	Name        string            `yaml:"name"`
	Transport   TransportType     `yaml:"transport"`
	Command     string            `yaml:"command,omitempty"`      // for stdio
	Args        []string          `yaml:"args,omitempty"`         // for stdio (passed after image for containers)
	Image       string            `yaml:"image,omitempty"`        // container image (for docker/podman)
	Volumes     []string          `yaml:"volumes,omitempty"`      // container volume mounts (host:container)
	RuntimeArgs []string          `yaml:"runtime_args,omitempty"` // extra flags passed to the container runtime (before the image)
	URL         string            `yaml:"url,omitempty"`          // for sse / streamable-http
	Headers     map[string]string `yaml:"headers,omitempty"`      // for remote transports
	Env         map[string]string `yaml:"env,omitempty"`          // for stdio (extra env vars)
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
// It prepends "run --rm -i", adds -e flags for env vars, -v flags for volumes,
// any extra runtime_args, the image, then the user args.
func resolveContainerArgs(cfg *ServerInstanceConfig) {
	args := make([]string, 0,
		3+2*len(cfg.Env)+2*len(cfg.Volumes)+len(cfg.RuntimeArgs)+1+len(cfg.Args))
	args = append(args, "run", "--rm", "-i")

	for k, v := range cfg.Env {
		args = append(args, "-e", k+"="+v)
	}

	for _, v := range cfg.Volumes {
		args = append(args, "-v", v)
	}

	args = append(args, cfg.RuntimeArgs...)
	args = append(args, cfg.Image)
	args = append(args, cfg.Args...)

	cfg.Args = args
	cfg.Env = nil     // env is passed via -e, not process env
	cfg.Volumes = nil // volumes are passed via -v
	cfg.RuntimeArgs = nil
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
