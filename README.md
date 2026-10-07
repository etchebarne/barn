# openbot

Self-hosted, single-user platform for persistent AI agents. Each agent is a named coworker
with its own memory, tasks, connectors and sandbox. You talk to agents in DMs and group chats,
and everything, including creating new agents, happens through chat.

Status: early development (milestone 1 of 8). See [docs/design.md](docs/design.md).

## Install

On a Linux server with systemd (amd64 or arm64):

```bash
curl -fsSL https://raw.githubusercontent.com/etchebarne/openbot/main/scripts/install.sh | bash
```

The installer installs missing prerequisites, downloads the latest release (verifying its
checksum), and runs openbot as a hardened systemd service under its own `openbot` user. Then open openbot
and follow the onboarding: create your account, paste your
[OpenCode Go](https://opencode.ai/docs/go/) API key, and create your first agent.

By default openbot listens on your Tailscale IP if Tailscale is installed, otherwise only on
`127.0.0.1:8080` (the installer prints the SSH tunnel command to reach it). To install Tailscale
and put openbot on your tailnet:

```bash
curl -fsSL https://raw.githubusercontent.com/etchebarne/openbot/main/scripts/install.sh | bash -s -- --tailscale
```

Other options: `--version v0.1.0` and `--addr host:port`. Rerun the installer to upgrade; your
config (`/etc/openbot/openbot.env`) and data (`/var/lib/openbot`) are kept.

Uninstall (keeps your data unless you add `--purge`):

```bash
curl -fsSL https://raw.githubusercontent.com/etchebarne/openbot/main/scripts/uninstall.sh | bash
```

## Development

Requires Go 1.26+, Node 22+ and pnpm 10.

```bash
pnpm install
cp .env.example .env   # server settings only; adjust if needed
```

Run the server and the web dev server in two terminals:

```bash
set -a; . ./.env; set +a; OPENBOT_WEB_DIR= go run ./cmd/openbotd
```

```bash
pnpm dev
```

Open http://localhost:5173. The Vite dev server proxies `/api` (including the WebSocket) to
`openbotd` on `127.0.0.1:8080`. First run walks you through creating your account, connecting
OpenCode Go and creating your first agent.

### Checks

```bash
pnpm lint && pnpm format:check && pnpm typecheck && pnpm test
go vet ./... && go test -race ./...
```

### API changes

`api/openapi.yaml` is the source of truth for both sides. After editing it:

```bash
go generate ./internal/api/gen/ && pnpm gen:api
```

## Releasing

Push a `v*` tag. The release workflow builds `openbotd` with the web app embedded for
linux/amd64 and linux/arm64 (`scripts/build-release.sh`) and publishes the archives and
checksums that the installer downloads.
