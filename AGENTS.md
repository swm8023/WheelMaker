# WheelMaker — AI Agent Entry Point

WheelMaker bridges local AI coding CLIs (Codex, Claude, etc.) to remote IM channels
(Feishu, mobile App), so developers can control their local AI assistant from a phone.

## Repo Layout

```
server/   — Go daemon: ACP bridge + IM adapters (Feishu, mobile WebSocket, console)
app/      — Flutter mobile app (iOS / Android)
docs/     — Protocol & design docs shared by both sides
```

## Universal Constraints

| Rule | Detail |
|------|--------|
| Language | Code comments & identifiers: **English only** |
| Commit discipline | After **every** change: `git add` → `git commit` → `git push` |
| Shared docs | Changes to `docs/` may affect **both** server and app — assess both sides |

## Mandatory Preflight (every session)

1. Read [`CLAUDE.md`](CLAUDE.md) — global architecture & full conventions.
2. Choose your track and read its guide:
   - **Go server work** → [`server/AGENTS.md`](server/AGENTS.md)
   - **Flutter app work** → [`app/AGENTS.md`](app/AGENTS.md)
3. Confirm before acting: `READ_OK: CLAUDE.md + <track>/AGENTS.md`

> Codex: AGENTS.md files are read automatically at each directory level.
> Claude: follow the explicit read-chain above before touching any code.

