# muxcp

**MCP server multiplexer** — aggregate multiple [Model Context Protocol](https://modelcontextprotocol.io/) servers behind a single endpoint.

## The problem

MCP clients (like Claude Code, Cursor, or custom agents) connect to MCP servers one at a time. If you need tools from multiple servers, you configure each one separately. This creates several issues:

- **No multi-instance support.** You can't run the same MCP server twice with different credentials. For example, you might need two Grafana MCP servers — one for production and one for staging — but your client only allows one entry per server type.
- **Configuration sprawl.** Each client needs its own copy of every server's connection details, environment variables, and credentials.
- **No credential isolation.** When running containerized MCP servers, you have to manually construct `docker run` commands with `-e` flags, volume mounts, and image names scattered across your client config.

## What muxcp does

muxcp sits between your MCP client and your MCP servers. It:

1. **Connects to multiple upstream MCP servers** — local processes (STDIO), containers (Docker/Podman), or remote servers (SSE/Streamable HTTP).
2. **Discovers all tools** from each server at startup.
3. **Namespaces every tool** as `{server_name}__{tool_name}` so there are no collisions.
4. **Exposes a single MCP endpoint** that your client connects to, presenting all tools from all servers as one unified set.

```
┌─────────────┐         ┌──────────┐       ┌─────────────────────┐
│             │  stdio   │          │ stdio │ grafana_prod (docker)│
│ Claude Code ├─────────►│  muxcp   ├──────►│ grafana_stg  (docker)│
│ / Cursor    │          │          │ http  │ github_work  (uvx)   │
│ / Agent     │          │          ├──────►│ remote_tools (SSE)   │
└─────────────┘         └──────────┘       └─────────────────────┘
```

Your client sees tools like `grafana_prod__search_dashboards`, `grafana_stg__search_dashboards`, `github_work__create_issue` — all through a single connection.

## Installation

### Homebrew (macOS/Linux)

```bash
brew tap moray95/tap
brew install muxcp
```

### Go install

```bash
go install github.com/moray95/muxcp/cmd/muxcp@latest
```

Make sure `$GOPATH/bin` (typically `~/go/bin`) is in your `PATH`.

### Pre-built binaries

Download the latest release for your platform from [GitHub Releases](https://github.com/moray95/muxcp/releases).

Available for:
- Linux (amd64, arm64, armv7)
- macOS (amd64, arm64)
- Windows (amd64, arm64)
- FreeBSD (amd64, arm64, armv7)

### Build from source

```bash
git clone https://github.com/moray95/muxcp.git
cd muxcp
make build
```

## Quick start

1. Create a config file at `~/.config/muxcp/config.yaml`:

```yaml
transport: stdio

servers:
  - name: github
    transport: stdio
    command: uvx
    args: ["mcp-server-github"]
    env:
      GITHUB_TOKEN: ${GITHUB_TOKEN}
```

2. Test it:

```bash
muxcp
```

3. Add it to your MCP client (see [Client setup](#client-setup) below).

## Configuration

muxcp uses a YAML config file. It looks for the config in the following locations (in order of precedence):

1. Path specified with `-config` flag
2. `./config.yaml` (current directory)
3. `~/.config/muxcp/config.yaml` (macOS/Linux) or `%AppData%\muxcp\config.yaml` (Windows)
4. `/etc/muxcp/config.yaml` (Unix) or `%ProgramData%\muxcp\config.yaml` (Windows)

Environment variables can be referenced as `${VAR_NAME}` anywhere in the config file.

See [config.yaml](config.yaml) for a fully annotated example with all supported patterns.

### Gateway settings

```yaml
# Transport the gateway exposes to clients: stdio, sse, or streamable-http.
# Use stdio when connecting directly from Claude Code, Cursor, etc.
transport: stdio

# Address to bind when using sse or streamable-http (ignored for stdio).
listen: ":8080"

# Externally-reachable URL for the SSE message endpoint.
# Useful when behind a reverse proxy with TLS.
# Defaults to http://localhost:{port}.
base_url: "https://mcp.example.com"
```

### Server entries

Each entry in `servers` defines an upstream MCP server. Every server must have a unique `name` — this becomes the namespace prefix for its tools.

#### Local process (STDIO)

Spawn a local command and communicate over stdin/stdout:

```yaml
servers:
  - name: github
    transport: stdio
    command: uvx
    args: ["mcp-server-github"]
    env:
      GITHUB_TOKEN: ${GITHUB_TOKEN}
```

| Field       | Required | Description                                    |
|-------------|----------|------------------------------------------------|
| `name`      | yes      | Unique identifier, used as tool namespace prefix |
| `transport` | yes      | Must be `stdio`                                |
| `command`   | yes      | Command to execute                             |
| `args`      | no       | Arguments passed to the command                |
| `env`       | no       | Extra environment variables for the process    |

#### Container (Docker / Podman)

When `command` is `docker`, `podman`, `nerdctl`, or `finch`, muxcp automatically constructs the container run command. You specify the `image`, `env`, `volumes`, and any extra `runtime_args` — muxcp adds `run --rm -i`, `-e`, and `-v` flags for you.

```yaml
servers:
  - name: grafana_production
    transport: stdio
    command: docker
    image: grafana/mcp-grafana
    args: ["-t", "stdio"]
    env:
      GRAFANA_URL: https://grafana.production.example.com
      GRAFANA_API_KEY: ${GRAFANA_PROD_API_KEY}

  - name: gpu_tools
    transport: stdio
    command: docker
    image: my-gpu-mcp-server
    runtime_args: ["--gpus", "all", "--network", "host"]
    volumes:
      - "/models:/models:ro"
      - "/data:/data"
    env:
      MODEL_PATH: /models/latest
```

This is equivalent to running:

```bash
docker run --rm -i \
  -e GRAFANA_URL=https://grafana.production.example.com \
  -e GRAFANA_API_KEY=... \
  grafana/mcp-grafana -t stdio

docker run --rm -i \
  -e MODEL_PATH=/models/latest \
  -v /models:/models:ro \
  -v /data:/data \
  --gpus all --network host \
  my-gpu-mcp-server
```

| Field          | Required | Description                                                                       |
|----------------|----------|-----------------------------------------------------------------------------------|
| `name`         | yes      | Unique identifier                                                                 |
| `transport`    | yes      | Must be `stdio`                                                                   |
| `command`      | yes      | Container runtime (`docker`, `podman`, `nerdctl`, `finch`)                        |
| `image`        | yes      | Container image to run                                                            |
| `args`         | no       | Arguments passed to the container entrypoint (after the image)                    |
| `env`          | no       | Environment variables passed via `-e` flags                                       |
| `volumes`      | no       | Volume mounts passed via `-v` flags (e.g. `/host/path:/container/path:ro`)        |
| `runtime_args` | no       | Extra runtime flags inserted before the image (e.g. `--gpus all`, `--network host`) |

#### Remote server (SSE)

Connect to a remote MCP server using Server-Sent Events:

```yaml
servers:
  - name: remote_sse
    transport: sse
    url: "https://mcp.example.com/sse"
    headers:
      Authorization: "Bearer ${REMOTE_API_KEY}"
```

#### Remote server (Streamable HTTP)

Connect to a remote MCP server using the Streamable HTTP transport:

```yaml
servers:
  - name: remote_http
    transport: streamable-http
    url: "https://mcp.example.com/mcp"
    headers:
      Authorization: "Bearer ${REMOTE_API_KEY}"
      X-Tenant-ID: "my-org"
```

| Field       | Required | Description                              |
|-------------|----------|------------------------------------------|
| `name`      | yes      | Unique identifier                        |
| `transport` | yes      | `sse` or `streamable-http`               |
| `url`       | yes      | Server endpoint URL                      |
| `headers`   | no       | HTTP headers (e.g., for authentication)  |

### Multi-instance pattern

The key feature of muxcp is running the same server multiple times with different credentials. Each instance gets a unique name, and tools are namespaced accordingly:

```yaml
servers:
  - name: github_personal
    transport: stdio
    command: uvx
    args: ["mcp-server-github"]
    env:
      GITHUB_TOKEN: ${GITHUB_PERSONAL_TOKEN}

  - name: github_work
    transport: stdio
    command: uvx
    args: ["mcp-server-github"]
    env:
      GITHUB_TOKEN: ${GITHUB_WORK_TOKEN}
```

The client will see `github_personal__create_issue` and `github_work__create_issue` as separate tools, each using their respective tokens.

## Tool namespacing

All tools are exposed with the pattern `{server_name}__{tool_name}` (double underscore separator). The tool description is also prefixed with `[{server_name}]` for clarity.

For example, a Grafana server named `grafana_prod` with a tool called `search_dashboards` becomes:

- **Tool name:** `grafana_prod__search_dashboards`
- **Description:** `[grafana_prod] Search for dashboards by title or tag`

## Usage

```bash
# Use default config file lookup
muxcp

# Specify a config file
muxcp -config /path/to/config.yaml
```

## Client setup

### Claude Code

```bash
claude mcp add muxcp -- muxcp
```

Or add to `.mcp.json` in your project root:

```json
{
  "mcpServers": {
    "muxcp": {
      "command": "muxcp",
      "args": ["-config", "/path/to/config.yaml"]
    }
  }
}
```

### Claude Desktop

Edit the config file:
- **macOS:** `~/Library/Application Support/Claude/claude_desktop_config.json`
- **Windows:** `%APPDATA%\Claude\claude_desktop_config.json`
- **Linux:** `~/.config/Claude/claude_desktop_config.json`

```json
{
  "mcpServers": {
    "muxcp": {
      "command": "muxcp",
      "args": ["-config", "/path/to/config.yaml"]
    }
  }
}
```

Restart Claude Desktop after editing.

### Cursor

Add to `.cursor/mcp.json` in your project root, or configure via Cursor Settings > MCP:

```json
{
  "mcpServers": {
    "muxcp": {
      "command": "muxcp",
      "args": ["-config", "/path/to/config.yaml"]
    }
  }
}
```

### VS Code (GitHub Copilot)

Add to `.vscode/mcp.json` in your workspace:

```json
{
  "servers": {
    "muxcp": {
      "type": "stdio",
      "command": "muxcp",
      "args": ["-config", "/path/to/config.yaml"]
    }
  }
}
```

> **Note:** VS Code uses `"servers"` as the top-level key, not `"mcpServers"`.

### Windsurf

Edit `~/.codeium/windsurf/mcp_config.json`:

```json
{
  "mcpServers": {
    "muxcp": {
      "command": "muxcp",
      "args": ["-config", "/path/to/config.yaml"]
    }
  }
}
```

### Zed

Edit `~/.config/zed/settings.json`:

```json
{
  "context_servers": {
    "muxcp": {
      "command": {
        "path": "muxcp",
        "args": ["-config", "/path/to/config.yaml"]
      }
    }
  }
}
```

> **Note:** Zed uses `"context_servers"` and nests the command under `"command.path"`.

### ChatGPT Desktop

ChatGPT Desktop supports MCP servers via Developer Mode. Enable it in Settings > Permissions & Roles, then add muxcp through the Developer Mode settings panel. ChatGPT primarily supports remote MCP servers (SSE/Streamable HTTP) — to use muxcp, configure it with `transport: sse` or `transport: streamable-http` and point ChatGPT to the gateway URL.

### Other clients

For any MCP client that supports STDIO transport, the pattern is the same: set the command to `muxcp` and optionally pass `-config /path/to/config.yaml` as an argument.

For clients that only support remote connections, run muxcp with `transport: sse` or `transport: streamable-http` and connect to the gateway URL.

## Testing your setup

You can use the [MCP Inspector](https://github.com/modelcontextprotocol/inspector) to interactively test your gateway:

```bash
npx -y @modelcontextprotocol/inspector muxcp -config config.yaml
```

This opens a web UI where you can browse all namespaced tools and call them interactively.

## Development

```bash
make build    # Build the binary
make run      # Build and run with config.yaml
make lint     # Run golangci-lint
make test     # Run tests with coverage (80% threshold)
make check    # Lint + test
make fix      # Auto-fix lint issues
make fmt      # Format code
make clean    # Remove binary and coverage output
```

## License

MIT
