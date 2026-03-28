# App Working Rules

## Web Dev Server
- After any release or local debug cycle, always restart the web dev server with port cleanup.
- Use this command from `app/`:
  - `npm run web:restart`

## What `web:restart` does
1. Kills all processes listening on port `8080`.
2. Starts the web dev server on port `8080`.

## Manual fallback
- If needed, run:
  - `powershell -NoProfile -ExecutionPolicy Bypass -File scripts/restart_web.ps1 -Port 8080`
