# barn

Self-hosted, single-user platform for persistent AI agents. Each agent is a named coworker
with its own memory, tasks, connectors and sandbox. You talk to agents in DMs and group chats,
and everything, including creating new agents, happens through chat.

Status: early development (milestone 1 of 8). See [docs/design.md](docs/design.md).

## Requirements

- Go 1.26+
- Node 22+ and pnpm 10
- An [OpenCode Go](https://opencode.ai/docs/go/) subscription (the API key is entered in the app)

## Development

```bash
pnpm install
cp .env.example .env   # server settings only; adjust if needed
```

Run the server and the web dev server in two terminals:

```bash
set -a; . ./.env; set +a; BARN_WEB_DIR= go run ./cmd/barnd
```

```bash
pnpm dev
```

Open http://localhost:5173. The Vite dev server proxies `/api` (including the WebSocket) to
`barnd` on `127.0.0.1:8080`. First run walks you through creating your account, connecting
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

## Running on a server

```bash
pnpm build
go build -o bin/barnd ./cmd/barnd
BARN_WEB_DIR=./web/dist BARN_ADDR=100.x.y.z:8080 ./bin/barnd
```

Bind `BARN_ADDR` to your Tailscale IP to keep barn private to your tailnet. State lives in
`BARN_DATA_DIR` (default `./data`): `barn.db` (SQLite) and `secret.key`, which encrypts stored
credentials. Back up both, and keep them separate if you can.

If you serve barn over HTTPS, set `BARN_SECURE_COOKIES=true`.
