# webterm

Self-hosted browser terminal with multi-session tabs, modern UI, and cross-platform metrics.

## Features

- Multiple terminal sessions with tabs, detach, and keyboard shortcuts
- Cross-platform CPU/memory/GPU metrics (macOS, Linux, Windows)
- Session snapshots and restore support
- Plain password auth + rate limiting
- Mobile-friendly UI

## Requirements

- Go 1.26+
- Node 20+ (for frontend build)

## Quick Start

Build and run:

```bash
make build
./webterm serve --config webterm.yaml
```

## Configuration

Generate a default config:

```bash
./webterm config init --path webterm.yaml
```

Key settings include:

- `server.bind` / `server.port`
- `auth.mode` and `auth.password`
- `terminal.shell` and `terminal.working_dir`
- `sessions.max_sessions` and snapshot settings

## Authentication

Set a plain password directly in `webterm.yaml`:

```yaml
auth:
  mode: password
  password: "your-password"
```

Password auth only.

## CLI Commands

- `webterm serve` — start the server
- `webterm doctor` — environment diagnostics
- `webterm config init` — create config template
- `webterm version` — build metadata
- `webterm completion [bash|zsh|fish]` — shell completions

## Development

```bash
make test
```

## Expose It (Tunnel Options)

Cloudflare Tunnel:

```bash
cloudflared tunnel --url http://localhost:8080
```

ngrok:

```bash
ngrok http 8080
```

Tailscale Serve:

```bash
tailscale serve --http=8080 8080
```

## Notes

`sessions.snapshot_key` accepts a simple string; it is derived into an encryption key automatically. Leave it empty to disable snapshot encryption.
The frontend build output is served from `frontend/out` and embedded into the Go binary at build time.
