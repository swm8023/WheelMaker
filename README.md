# WheelMaker
WheelMaker â€“ A Go library that turns your phone into a remote AI coding assistant. 
Stop reinventing the wheel, start making your own. ðŸ›ž

A bridge daemon that connects local AI coding CLIs (Codex, Claude, etc.) to IM platforms like Feishu, letting you remotely control your local AI assistant from your phone.

```
Feishu (mobile) â”€â”€â–º WheelMaker â”€â”€â–º codex-acp / claude-acp â”€â”€â–º your codebase
```

## Features

- **Multi-project**: manage multiple projects in one process, each with its own IM and AI agent
- **Feishu integration**: send and receive messages via Feishu Bot, with rich card support
- **Console mode**: use stdin instead of Feishu for local testing
- **Session persistence**: automatically resumes the previous AI session after restart (via `session/load`)
- **Lazy loading**: AI subprocess starts on the first message; auto-closes after 30 min idle to save resources
- **Multiple backends**: Adapter abstraction supports codex-acp today, extensible to any ACP-compatible CLI

## Quick Start

### 1. Install tool binaries

```bash
# Linux/macOS
./scripts/install-tools.sh

# Windows
./scripts/install-tools.ps1
```

### 2. Create a config file

```bash
cp config.example.json ~/.wheelmaker/config.json
```

Edit `~/.wheelmaker/config.json`:

```json
{
  "projects": [
    {
      "name": "my-project",
      "im": { "type": "console" },
      "client": { "adapter": "codex", "path": "/path/to/your/code" }
    }
  ]
}
```

### 3. Run

```bash
export OPENAI_API_KEY=sk-...
go run ./cmd/wheelmaker/
```

A prompt will appear â€” type messages directly:

```
[my-project] > write an HTTP health check endpoint
```

## Configuration

Config file: `~/.wheelmaker/config.json`

### IM types

**console** (local testing):
```json
{ "type": "console", "debug": true }
```
`debug: true` prints all ACP JSON to stderr for protocol-level debugging.

**feishu** (production):
```json
{ "type": "feishu", "appID": "cli_xxxxxxxx", "appSecret": "your_app_secret" }
```

### Multi-project example

```json
{
  "projects": [
    {
      "name": "backend",
      "im": { "type": "feishu", "appID": "cli_xxx", "appSecret": "yyy" },
      "client": { "adapter": "codex", "path": "/home/user/backend" }
    },
    {
      "name": "frontend",
      "im": { "type": "console" },
      "client": { "adapter": "codex", "path": "/home/user/frontend" }
    }
  ],
  "feishu": { "verificationToken": "your_verification_token" }
}
```

## Commands

Send in IM or console:

| Command | Description |
|---------|-------------|
| `/use <adapter>` | Switch AI backend (e.g. `/use codex`) |
| `/use <adapter> --continue` | Switch and carry over current context |
| `/cancel` | Cancel the in-progress request |
| `/status` | Show current adapter and session ID |
| anything else | Sent to the AI as a message (including text starting with `/`) |

## Architecture

```
Hub
â””â”€ client.Client (per project)
     â”œâ”€ im.Adapter        â† Feishu / Console
     â””â”€ agent.Agent       â† ACP protocol layer
          â””â”€ provider.Provider â†’ acp.Conn â†’ codex-acp binary
```

| Package | Responsibility |
|---------|----------------|
| `internal/hub/` | Reads config, manages lifecycle of all project clients |
| `internal/client/` | Per-project coordination: routing, lazy agent init, idle timeout, state persistence |
| `internal/agent/` | ACP session lifecycle, streaming prompts, fs/terminal/permission callbacks |
| `internal/agent/provider/` | JSON-RPC 2.0 over stdio; owns subprocess lifetime |
| `internal/agent/provider/codex/` | Stateless connection factory â€” launches codex-acp binary |
| `internal/im/console/` | Console IM: reads stdin; optionally logs all ACP JSON to stderr |
| `internal/im/feishu/` | Feishu Bot IM adapter |

## Development

```bash
go test ./...

# Integration tests (requires real codex-acp binary)
go test -tags integration ./internal/agent/provider/codex/...

go build ./cmd/wheelmaker/
```

Runtime state (session IDs) is persisted to `~/.wheelmaker/state.json` automatically.

## Protocol Docs

- [ACP Protocol](docs/acp-protocol-full.zh-CN.md)
- [Feishu Bot Setup](docs/feishu-bot.md)
- [codex-acp Reference](docs/codex-acp.md)





