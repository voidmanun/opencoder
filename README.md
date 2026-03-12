# DingTalk-Bridge

A bridge service connecting DingTalk chatbot to OpenCode AI assistant with real-time streaming response support.

## Features

- **DingTalk Stream Integration**: Receive messages via DingTalk Stream mode (no public IP required)
- **Interactive Card Streaming**: Real-time typewriter effect using DingTalk interactive cards
- **Session Management**: Persistent sessions with configurable timeout (default 8 hours)
- **Tool Whitelist**: Security-focused tool restriction via TypeScript plugin
- **Graceful Shutdown**: Clean termination with signal handling

## Architecture

```
┌─────────────────┐     ┌──────────────────┐     ┌─────────────────┐
│   DingTalk      │     │ dingtalk-bridge  │     │   OpenCode      │
│   (Stream SDK)  │◄───►│     (Go)         │◄───►│   Server API    │
└─────────────────┘     └──────────────────┘     └─────────────────┘
                               │
                               ▼
                        ┌──────────────────┐
                        │   Plugin         │
                        │ (TypeScript)     │
                        └──────────────────┘
```

## Prerequisites

1. **Go 1.23+** - Install from https://go.dev/
2. **OpenCode CLI** - Install from https://opencode.ai/
3. **DingTalk Bot App** - Create at https://open-dev.dingtalk.com

## Quick Start

### 1. Install Go

```bash
brew install go
```

### 2. Clone and Setup

```bash
git clone <your-repo-url>
cd dingtalk-bridge
cp .env.example .env
```

### 3. Configure Environment

Edit `.env` with your credentials:

```bash
# DingTalk App Credentials (from developer console)
DINGTALK_CLIENT_ID=your_client_id
DINGTALK_CLIENT_SECRET=your_client_secret

# OpenCode Server
OPENCODE_SERVER_PASSWORD=your_secure_password
```

### 4. Start OpenCode Server

```bash
OPENCODE_SERVER_PASSWORD=your_secure_password opencode serve
```

### 5. Run the Bridge

```bash
make deps
make run
```

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DINGTALK_CLIENT_ID` | Yes | - | DingTalk app Client ID |
| `DINGTALK_CLIENT_SECRET` | Yes | - | DingTalk app Client Secret |
| `OPENCODE_SERVER_URL` | No | `http://127.0.0.1:4096` | OpenCode server URL |
| `OPENCODE_SERVER_PASSWORD` | Yes | - | OpenCode server password |
| `BRIDGE_MODE` | No | `advanced` | Streaming mode: `advanced` or `mvp` |
| `SESSION_TIMEOUT` | No | `28800` | Session timeout in seconds (8 hours) |
| `LOG_LEVEL` | No | `info` | Log level: debug, info, warn, error |

### Streaming Modes

| Mode | Description | Best For |
|------|-------------|----------|
| `advanced` | Interactive card streaming with typewriter effect | Best UX |
| `mvp` | Multi-message fallback | Compatibility |

### Tool Whitelist

Edit `config/tool_whitelist.json` to configure allowed/blocked tools:

```json
{
  "default_allowed": ["read", "glob", "grep"],
  "default_blocked": ["bash", "write", "edit"],
  "user_overrides": {
    "user_id_here": {
      "allowed": ["write", "edit"],
      "blocked": []
    }
  }
}
```

## Project Structure

```
dingtalk-bridge/
├── cmd/dingtalk-bridge/main.go    # Entry point
├── internal/
│   ├── config/config.go           # Configuration management
│   ├── logger/logger.go           # Structured logging
│   ├── dingtalk/
│   │   ├── client.go              # Stream SDK client
│   │   ├── card_client.go         # Interactive card API
│   │   ├── card_template.go       # Card templates
│   │   └── replier.go             # Text reply fallback
│   ├── opencode/
│   │   ├── server_client.go       # HTTP API client
│   │   └── sse_reader.go          # SSE event parser
│   ├── session/store.go           # Session persistence
│   └── bridge/router.go           # Message routing
├── plugins/
│   └── dingtalk-guard.ts          # TypeScript guard plugin
├── config/
│   ├── tool_whitelist.json        # Tool whitelist config
│   └── user_whitelist.json        # User whitelist config
├── Makefile
├── go.mod
├── .env.example
└── README.md
```

## Development

### Build

```bash
make build
```

### Test

```bash
make test
```

### Run with Debug Logging

```bash
LOG_LEVEL=debug make run
```

## Creating a DingTalk Bot

1. Go to https://open-dev.dingtalk.com
2. Create a new application
3. Enable "机器人" (Bot) feature
4. Copy Client ID and Client Secret to `.env`
5. Configure message receiving mode: Stream

## Security Considerations

- **Localhost Only**: Bridge binds to `127.0.0.1` only
- **Password Protected**: OpenCode server requires authentication
- **Tool Whitelist**: Dangerous tools blocked by default
- **User Whitelist**: Optional DingTalk user restrictions

## Troubleshooting

### Connection Failed

1. Verify DingTalk credentials are correct
2. Check network connectivity to `api.dingtalk.com` and `wss-open-connection.dingtalk.com`
3. Ensure OpenCode server is running

### Card Streaming Not Working

1. Check if `BRIDGE_MODE=advanced`
2. Verify DingTalk app has card permissions
3. Check logs for API errors

### Tools Being Blocked

1. Review `config/tool_whitelist.json`
2. Add user-specific overrides if needed
3. Check plugin is loaded correctly

## License

MIT

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Run tests
5. Submit a pull request