# openbot — design

Self-hosted, single-user platform for persistent AI agents. Each agent is a named "coworker" with
its own sandbox, memories, tasks, and connectors. You talk to agents in DMs and group chats, and
everything — including creating new agents — happens through chat.

Status: draft, pre-implementation.

## 1. Goals and non-goals

**Goals (v1)**
- Run on one VPS, for one user, reachable over Tailscale (public internet optional).
- Use an OpenCode Go subscription as the only model provider.
- Agents are long-lived, act autonomously inside their own sandbox, and reach out to the user on
  their own (scheduled tasks, connector signals).
- One chat UI for web, mobile (PWA), and desktop (Electron wrapping the same web app).
- Open source, easy to self-host (`docker compose up`).

**Non-goals (v1)**
- Multi-user / multi-tenant.
- Native mobile apps.
- Model providers other than OpenCode Go (keep the client OpenAI-compatible so this is cheap later).
- Agent-to-agent DMs (agents coordinate only in group chats the user creates).

## 2. Concepts

| Concept | Description |
|---|---|
| **Agent** | A persistent mind: name, purpose/instructions, personality (how it comes across, apart from what it does and what it remembers; the agent edits it when asked to talk differently), model, language, notification preference, trust mode. Owns its tasks, memories, and sandbox assignment. |
| **Chat** | Either a **DM** (user ↔ one agent) or a **group** (user + chosen agents). Single-threaded forever; never "new session". |
| **Message** | Something posted in a chat by the user, an agent, or the system. |
| **Event** | Anything that wakes an agent: a message it can see, a task firing, a connector signal, an approval answer, a tool result from a parked action. |
| **Context** | One per agent, across all its chats. Events are tagged with their origin so the agent knows where it heard what. Compacted when it nears the model's limit. |
| **Memory** | A durable fact/preference the agent saved ("martin prefers lowercase replies"). Private to the agent. If learned in a group, each agent that chose to remember it stores its own copy. |
| **Task** | A trigger owned by an agent: `cron` (schedule), `signal` (connector event matching a filter), or `once` (a single future time). When it fires, the agent receives an event with the task's purpose. |
| **Connector account** | One authenticated account on an external service (e.g. "Slack — work", "Slack — side project"). Provides **tools** (actions) and **signals** (incoming events). |
| **Grant** | Gives an agent access to a connector account. One account can be granted to several agents. |
| **Sandbox** | A Docker container where the agent runs commands. One per agent by default; can be shared by several agents. All sandboxes mount `/shared`. |
| **Prompt** | An interactive message from an agent: an approval (approve/decline), a multiple-choice question, or a free-text question. |

## 3. Architecture

```
 ┌──────────────┐   ┌──────────────┐
 │ Web / PWA    │   │ Electron     │      (same React app)
 └──────┬───────┘   └──────┬───────┘
        │ HTTPS + WebSocket │
 ┌──────▼──────────────────▼────────────────────────────────────────┐
 │ openbotd (Go, single binary)                                        │
 │                                                                  │
 │  API (REST + WS) ── Auth                                         │
 │        │                                                         │
 │  Event bus ──► Agent runtime (one goroutine + inbox per agent)   │
 │   ▲   ▲            │  model client ──────────► OpenCode Go       │
 │   │   │            │  tools ──► Sandbox manager ──► Docker       │
 │   │   │            │        └─► Connector manager ──► Slack, …   │
 │   │   └── Scheduler (cron / once)                                │
 │   └────── Connector signal ingress (webhooks, sockets, polling)  │
 │                                                                  │
 │  Group coordinator    Push notifier (Web Push / Electron)        │
 │                                                                  │
 │  Store: SQLite (WAL)                                             │
 └──────────────────────────────────────────────────────────────────┘
```

`openbotd` runs on the host (or in a container with access to the Docker socket). Agents never run
inside `openbotd`'s process space for commands; all execution goes through the sandbox manager.

## 4. Agent runtime

### 4.1 One loop per agent
Each agent has an **inbox** (persistent queue in SQLite, mirrored in memory) and a single runtime
goroutine. The goroutine processes one **turn** at a time:

1. Drain all pending events from the inbox.
2. Append them to the agent's context as tagged entries, e.g.
   `[dm] martin: can you check the deploy?` or `[task: Weekday check-in] fired at 10:01`.
3. Call the model with the assembled context and the agent's tools.
4. Execute tool calls. **Between every tool call, drain the inbox again** and inject any new events
   before the next model call. This is how a busy agent can notice a DM mid-job and reply
   ("on it, almost done") without abandoning its work.
5. The turn ends when the model returns without tool calls.

Events arriving while no turn is running start a new turn immediately.

### 4.2 Speaking is a tool
The model's plain text output is **private reasoning** and is never shown in a chat. To say
anything, the agent calls `send_message(chat, text)`. This follows from "one mind, many chats":
the agent chooses where to speak (DM, a specific group, or nowhere), which is what makes
"stop posting in the group, DM me instead" work naturally.

The system prompt lists the chats the agent belongs to and their participants.

**Replies.** A message can reply to an earlier message in the same chat (the user from the
message's Reply action, an agent with `send_message(…, reply_to)`). Clients show the start of
the original above the reply; agents get it in a `<replying_to message_id from>` block at the
top of the message, so "yes, this one" is unambiguous even in a busy group.

### 4.3 Context assembly and compaction
Each model call is built from:

1. **System prompt**: identity, purpose/instructions, personality, language, trust mode, the date,
   chat roster, its tasks, granted connectors, sandbox info, and all its memories
   (memories are expected to stay small; add retrieval later if needed).
2. **Summary** of older history (from previous compactions).
3. **Recent event log**: all chats interleaved chronologically, origin-tagged, plus tool calls
   and results. Every event carries when it was sent or received.

**Keeping requests cheap.** Every step resends the whole context, so:
- *Prompt caching.* Providers cache a request's unchanged start. The system prompt is frozen per
  agent (`agents.prompt_snapshot`) and rebuilt only when what it's built from changes
  (instructions, chats, tasks, connections, secrets, approvals: a fingerprint of everything but
  memories), after compaction, or when the user edits memories; memories an agent saves itself
  wait (it already knows them). It carries the date, not the time, and task lists have no
  next-run times. Anthropic-protocol requests mark the end of the system prompt and the last two
  messages with `cache_control`.
- *Tool output limits.* Results over 100k characters (50k for app tools, 200k per turn) are saved
  to a file in the agent's computer and replaced by a preview and the path; clipped command output
  is saved in full; `read_file` reads in pages of 2,000 lines / 100k characters and answers a
  reread of an unchanged file briefly.
- *Large app toolsets* (over 15 tools or ~4k tokens of definitions) load on demand: a one-line
  catalog in the prompt plus `app_tool_info` / `app_tool_call` (unwrapped into the real call, so
  approvals and limits apply).
- *Subagents.* `delegate` hands a job to a helper with its own fresh context (the agent's computer
  and read-only apps; no messaging, approvals, scheduling or further delegation); only its report
  (≤24k characters) comes back. Several delegate calls in one step run in parallel.
- *Usage.* Every model call's tokens (input, cached, cache writes, output, reasoning) are recorded
  per agent and purpose (`model_usage`) and shown per agent and in Settings.

**Compaction** triggers at half the model's context window (from models.dev), capped at 64k tokens
(`OPENBOT_COMPACT_AT_TOKENS`), measured with estimates calibrated by the provider's real counts.
First, old tool results over 1,000 characters become one-line stubs (calls kept, so the context
stays valid); if that frees enough, no summary is written. Otherwise the oldest part is folded
into a structured summary (goals, done, in progress, promises and open questions per chat,
decisions, people, errors, files), keeping a verbatim tail of 2.5% of the window (10k–25k tokens,
at least 8 entries). Secrets are masked before summarizing. Full messages remain in the store;
only the agent's working context is compacted.

**Clearing a DM.** The user can clear their DM with an agent: its messages are deleted, and the
agent forgets that conversation. Between turns (never during one), its context loses each turn
the DM started, and each non-group turn that wrote to the DM (e.g. a task that messaged the user);
group turns stay. The summary of older context can't be split by chat, so it's dropped only for
agents in no groups. Saved memories, personality, settings and tasks are kept.

### 4.4 Built-in tools

| Tool | Purpose |
|---|---|
| `send_message(chat_id, text)` | Speak in a chat the agent belongs to. |
| `react(message_id, emoji)` | React instead of replying when a message doesn't need words. |
| `ask_user(chat_id, kind, question, options?)` | Clickable question (single / multi / text). Ends the turn; the answer arrives as an event. |
| `connect_app(type, name?, config?, agent_ids?, reason?)` | Propose a connection; the user adds secrets on the card. Ends the turn. |
| `memory_save`, `memory_forget` | Durable memories, always shown in the agent's instructions. The user can add, edit and delete them too, in the agent's settings. |
| `update_agent`, `list_agents`, `list_models` | Change own settings (admins: any agent's); find teammates and models. |
| `task_create`, `task_update`, `task_delete` | Schedule work: `at` (once), `cron` (repeating), or `on_signal` (connector events), optionally gated by a `check`. The user can create and edit `at`/`cron` tasks in the agent's settings (same validation). |
| `run_command`, `read_file`, `write_file`, `list_files` | The agent's sandbox (when Docker is available). |
| `<account>__<tool>` | Tools of connector accounts the agent was granted (or `app_tool_info` / `app_tool_call` when they're many). |
| `delegate(task, context?, model?)` | Hand a job to a helper with its own context; only its report comes back. |
| `request_secret(name, description)` | Ask the user for a secret through a secure field. Ends the turn. |
| `done` | End the turn. |
| `create_agent`, `delete_agent`, `create_group`, `update_group` | Admin agents only (the starter agent is admin). |

### 4.5 Approvals and trust
- Each agent has a **trust mode**: `ask` (default) or `trusted`.
- In `ask` mode, **gated** tool calls don't run: the runtime posts an Approve / Decline card in the
  agent's DM and ends its turn. Approving runs the stored call in the harness and delivers the
  result to the agent as an `<approval_result>`; declining tells it so. Gated: every connector
  tool marked external (posting, creating, deploying). `delete_agent` always asks, even for
  trusted agents, and can't be allowed for good: it can't be undone. `create_agent` isn't gated:
  the intro already asks "Want me to set up an agent for that?".
- Agents can still ask for confirmation themselves with `ask_user` whenever they're unsure.
- `trusted` mode skips the gate (turning it on is confirmed in the UI).

## 5. Group chats

Groups contain the user and an ordered list of agents. Coordination is **turn-based**:

1. A new message (from the user, or an agent posting on its own, e.g. after finishing a long job)
   starts a **cycle**.
2. The coordinator gives agents turns in order, starting after the author. A turn delivers a
   `group_turn` event containing every group message that agent hasn't seen yet.
3. On its turn the agent either calls `send_message` to the group or stays silent. It decides
   based on whether anything new was directed at it or relevant to its role.
4. The cycle continues round after round. It ends when **a full round passes with no messages**.
5. Safeguards:
   - Maximum rounds per cycle (default 8). When hit, the coordinator posts a system note and stops.
   - Turn timeout (default 60 s to *start* responding). A busy agent sees the turn between tool calls;
     if it doesn't respond in time it is skipped this round and catches up on its next turn.
   - A user message mid-cycle joins the cycle; agents see it on their next turn.
6. The "should I speak?" step is a full model call per agent per round. Optionally run that
   decision on a cheaper model and only invoke the agent's own model if it decides to reply.

**Mentions.** Anyone (the user or an agent) can tag participants with `@name`. Mentions don't
change the rules above; they make intent explicit:
- Mentioned agents take their turns **first** in the cycle (in mention order), then the
  normal order continues.
- The `group_turn` event flags messages that mention the receiving agent, so "was this for me?"
  doesn't depend on guessing.
- Agents can mention each other (e.g. `@issues-bot this is done`) to hand off work.
- An agent mentioning the user (`@martin`) sends a push notification, even if the group's
  notifications are otherwise quiet.

Messages in a group are visible to every participant; DMs are visible only to that agent.

## 6. Tasks and triggers

| Kind | Definition | Fires when |
|---|---|---|
| `cron` | 5-field cron, evaluated in the user's time zone | The schedule matches. |
| `once` | A date-time | It's reached (then the task is done). |
| `signal` | Connector account + signal type + `match` (field → text it must contain, case-insensitive) | A matching event arrives. |

- The web app reports the browser's time zone (`PUT /settings/timezone`); agents' "current time"
  and schedules use it.
- A single scheduler wakes at the next due time (at most every minute). Firing is an atomic
  claim on the task's `next_fire_at`, so a run can never fire twice; runs missed while the server
  was down collapse into one.
- Firing enqueues a `task_fired` event (with the signal's fields for signal tasks). Agents with
  `notifications` on get their messages pushed to the user's devices (Web Push, VAPID keys stored
  encrypted; the service worker skips the chat that's open).
- **Separate runs.** Task events run outside the agent's main conversation: the same frozen system
  prompt and summary (sharing their cache), a digest of the latest 10 DM messages, and the event.
  A quiet run leaves no trace. A run that messages or acts leaves a short `task_report` that joins
  the main conversation with the next real event (it doesn't start a turn), so the agent can
  follow up when the user replies. A burst of events for one task is one run.
- **Checks.** A cron or once task can have a `check`: a shell command run on the schedule in the
  agent's computer (with its secrets), at no token cost. The agent is woken only when the output
  differs from the previous run's (stored in `tasks.check_output`; the output at creation is the
  baseline), and then sees before and after.

## 7. Connectors

- **Types** (`internal/connectors`): Webhook (any JSON POST, secret token in the URL), GitHub
  (REST + `X-Hub-Signature-256` webhooks), Linear (GraphQL + `Linear-Signature` webhooks), Slack
  (Web API + Socket Mode, so no public URL is needed), Render (REST + Standard Webhooks
  signatures with a replay window), MCP servers over Streamable HTTP (JSON or SSE responses), and
  local MCP servers over stdio (`npx …`, `uvx …`). For both MCP kinds, tools the server marks
  read-only aren't gated.
- **Local MCP servers** run in a container of their own (`openbot-sbx-mcp`, always the default
  image with Node.js, Python and uv), never in an agent's sandbox: an agent could read the
  server's environment there, and that's where its secrets are. Secrets are entered as
  `KEY=value` pairs and handed to `docker exec -e KEY` through the client's environment, so they
  don't appear in the host's process list. Servers start on first use, are shared by the
  agents granted the account, answer the server's `ping` requests, stop after 10 idle minutes
  or when the account is edited or removed (the whole process tree inside the container), and
  report the end of their stderr when they crash. Tool lists are cached for an hour so an idle
  server can stay stopped between turns. Verifying allows 2 minutes for the first download, and
  openbotd builds the image in the background at startup.
- **Accounts** are added in Settings → Connectors, or proposed by an agent with `connect_app`:
  the agent fills in the type, name, non-secret config (e.g. an MCP server URL) and who gets
  access, and a connect card appears in its DM. The user types any secrets on the card
  (`POST /messages/{id}/connect`), so they never pass through chat or the model. Either way,
  credentials are verified with the service before saving and stored encrypted; the API never
  returns them. A failed check leaves the card open; the agent learns the outcome (and its new
  tool names) as a `<connection_result>`. Only admin agents may propose access for others.
  Several accounts per type are fine ("Slack — work", "Slack — side").
- **Sign-in** (MCP authorization spec): for MCP servers that answer 401 with OAuth metadata,
  openbot discovers the authorization server (RFC 9728 → RFC 8414), registers itself (dynamic
  client registration, RFC 7591), and sends the user to the authorize page with PKCE (S256)
  and a resource indicator. The browser comes back to `/oauth/callback`, which exchanges the
  code and redirects into the app. Services that refuse plain-http, non-loopback redirects
  (Linear, Notion) get the loopback redirect instead: the user lands on an error page and
  pastes its address into openbot (HTTPS for openbot, e.g. Tailscale Serve, avoids this). Tokens are
  stored encrypted with the account and refreshed shortly before expiry, or once when a call
  is rejected; a refused refresh marks the connection "sign-in expired" until Reconnect.
  Pending sign-ins are in memory, single use, and expire after 15 minutes. A catalog lists
  servers verified to support all of this (Linear, Notion, Sentry, Stripe); agents can
  propose them with `connect_app`, and the card then offers Sign in instead of key fields.
- **Grants** decide which agents may use an account. Tools appear to an agent as
  `<account slug>__<tool>`; the system prompt lists its accounts and their signal types.
- **Signals** arrive at `POST /hooks/{accountID}` (public, verified per type) or over a
  listener (Slack Socket Mode). They're stored, normalized to flat string fields, and matched
  against enabled `signal` tasks of agents granted that account.
- **Reaching webhooks**: set `OPENBOT_PUBLIC_URL` to a URL the service can reach (Tailscale Funnel
  for just `/hooks/*`, or a reverse proxy); the Settings UI shows the full webhook URL to paste.
- Connector content is untrusted input; external actions stay gated unless the agent is trusted.

## 7b. Attachments

- **Upload**: `POST /chats/{id}/attachments` (multipart, ≤ 25 MB) stores the file under
  `<data>/shared/attachments/<id>/<name>` (every sandbox sees it at `/shared/attachments/…`) and
  records it unsent; `SendMessageRequest.attachmentIds` (≤ 10) attaches uploads from the same chat
  to the message in one transaction. Unsent uploads and files without a record are cleaned up
  hourly (after a day). Types are sniffed from the content; image dimensions are recorded.
- **Serving**: `GET /api/attachments/{id}` needs the session. Only images (PNG, JPEG, GIF, WebP)
  and PDFs are served inline with their type; everything else is
  `application/octet-stream` + `attachment` (no HTML/SVG can run in openbot's origin), with
  `nosniff` and a sandboxing CSP.
- **Agents**: each attachment is listed as `<attachment name type size path/>`; small text files
  (≤ 32 KB) are inlined; images are shown to the model as image parts in all three protocols.
  Stored context keeps only image ids (`openbot_images`); bytes are loaded per request, for the 3
  most recent messages with images. A model that rejects images gets the request again without
  them (and a note that they're files), and is remembered as text-only.
- **Agents sending files**: `send_message` takes `files` (paths in the sandbox, or `/shared/…`
  read on the host after cleaning the path); they're copied out (binary-safe, size checked)
  and attached.

## 8. Sandboxes

- Docker, through the CLI. Each agent gets a long-lived container (`openbot-sbx-<id>`) on first
  use, from the `openbot-sandbox` image (Debian + git, curl, Python, Node, build tools, jq, ripgrep;
  built automatically from an embedded Dockerfile, or `OPENBOT_SANDBOX_IMAGE`). Agents are root
  inside and can install anything.
- `/home/agent` is a named volume (persists across restarts); the host's `data/shared` is mounted
  at `/shared` in every sandbox. Limits: 2 GB memory, 2 CPUs, 512 processes.
- `run_command` runs `bash -lc` under `timeout --signal=KILL` inside the container (default 2
  min, max 15), so runaway commands die; long output is clipped in the middle.
- Agents can share a sandbox (`sandbox_with` on create/update). Settings show its status and can
  restart it. Without Docker (`OPENBOT_SANDBOX=off` or not installed), the tools aren't offered and
  agents are told.
- The installer installs Docker and adds the `openbot` user to the `docker` group (root-equivalent on
  the host; `--no-docker` skips it).
- **Secrets.** Agents ask for API keys and tokens with `request_secret` (name like
  `GITHUB_TOKEN`, plus what it is): a card with a password field in their DM, so secrets never go
  through chat. The value is sealed with the server key in `agent_secrets` (the prompt only
  records "provided"), passed to `run_command` as an environment variable (through the docker
  client's environment, not its arguments), and masked as `[secret NAME]` in every tool result,
  so the model never sees it. The system prompt lists names only; agent settings list, replace
  and delete them.
- **The user's own access ("Computer").** The app opens an agent's sandbox directly: a file
  browser (list, preview, edit, upload, download, move, delete, up to 200 MB per file) and a real
  terminal (`docker exec -it … bash -l` on a host pseudo-terminal, streamed over a same-origin
  WebSocket; resizable). The shell runs as the agents do, its whole process tree ends when the
  connection closes, and the agent sees whatever the user changes.

## 9. Models

- OpenCode Go serves each model through one of three APIs: `/v1/chat/completions` (Kimi, GLM,
  DeepSeek, …), `/v1/responses` (GPT, Grok, Muse Spark) and the Anthropic-style `/v1/messages`
  (MiniMax, Qwen). `openbotd` keeps conversations in the chat-completions shape and translates per
  protocol in a small in-house client (`internal/model`). The protocol is guessed from the model
  id; if the provider says the model doesn't support it, the client falls back to the others and
  remembers the one that worked. Provider extensions such as `reasoning_content` are preserved.
- Every request sends `x-opencode-session` (the agent's id, since each agent is one continuous
  conversation) and identifies as `openbot/<version>`, as OpenCode Go requires.
- The API key is entered during onboarding (or replaced in Settings), verified with a one-token
  request, and stored in the `settings` table encrypted with AES-GCM. The encryption key lives in
  `data/secret.key` (0600), separate from the database, so a leaked DB backup alone doesn't leak it.
- Available models: `GET /models` on the provider (public, no key needed), cached for 10 minutes.
  It doesn't report context window sizes, so those come from config.
- Each agent has a `model`. On creation, the creating agent asks the user to pick one
  (`ask_user` with choices from `list_models`).
- Optional per-agent `utility_model` for compaction and group "should I speak?" checks.
- Context window size per model is in config (needed for compaction thresholds).

## 10. Clients

### 10.1 Product
- Single-page app served by `openbotd`.
- Layout: sidebar of DMs and groups (unread badges), chat pane, composer. Settings page for
  account, auth, appearance, and system config. Agents, tasks, memories, connectors, and
  sandboxes are managed by talking to agents, not with forms (read-only inspection views are fine).
- Composer supports `@` autocomplete of group participants; mentions render as chips.
- Messages arrive whole (agents speak via a tool call), so there is no text streaming. Instead, a
  live **activity line** shows what an agent is doing ("thinking…", "running `npm test`…").
- Theme: light / dark / system, **system by default**, switchable in settings and persisted
  per device.
- **PWA** with Web Push for mobile (iOS requires adding to the Home Screen).
- **Electron** shell (`desktop/`) loading the user's server, with native notifications, an unread
  badge and a tray icon:
  - First launch asks for the server address and checks it (`GET /api/auth/status`). The window
    only ever shows that origin (other links open in the browser), with context isolation,
    sandboxed renderers and a permission allowlist. Plain-http servers (Tailscale) are treated as
    secure contexts so clipboard and crypto APIs work.
  - A tiny preload bridge (`window.openbotDesktop`): `notify`, `setUnread`, `onNavigate`,
    `changeServer`. Web Push doesn't exist in Electron, so the web app notifies from its realtime
    connection instead (agents with notifications on, unless that chat is open and focused).
  - Closing keeps it running in the tray (configurable). It checks GitHub releases for updates and
    links to the download; builds for Linux (AppImage, deb, rpm), Windows and macOS (unsigned
    for now) are attached to each release.

### 10.2 Stack

| Concern | Choice |
|---|---|
| Build | Vite, React, TypeScript (`strict`) |
| UI | shadcn/ui on Tailwind v4, themed via CSS variables (`neutral` base for now) |
| Chat UI | shadcn chat components: `MessageScroller`, `Message`, `Bubble`, `Attachment`, `Marker`, `shimmer`/`scroll-fade` utilities. Composer built from `InputGroup` + `Textarea` + `Popover` (mentions) until an official one ships. Prompts render as `Bubble` actions. |
| Routing | TanStack Router, file-based, type-safe params and search |
| Server state | TanStack Query |
| Realtime | One WebSocket client that applies events to the Query cache (`setQueryData` / invalidation), so server data has a single source of truth |
| Client state | Zustand for UI-only state (drafts, sidebar, connection status) |
| API contract | OpenAPI spec in `api/openapi.yaml` is the source of truth: Go server stubs via `oapi-codegen`, TS types and client via `openapi-typescript` + `openapi-fetch`. WebSocket event payloads are defined in the same spec. |
| Message rendering | Markdown with code highlighting |
| Lint / format | oxlint (type-aware), oxfmt (beta, version pinned), `tsc --noEmit` |
| Tests | Vitest + Testing Library; Playwright e2e later |

### 10.3 Structure

```
web/src/
  app/            providers, router, query client, ws bootstrap, theme
  routes/         TanStack Router file routes (thin: compose features)
  features/
    auth/         login, passkeys, session
    chats/        sidebar, chat view, composer, mentions, activity line
    prompts/      approval / choice / text prompt bubbles
    agents/       avatars, status, inspection views
    onboarding/   first-run: connect OpenCode Go, create the starter agent
    settings/     appearance (theme), model provider, account
  components/ui/  shadcn-generated (owned, tweak sparingly)
  lib/            generated api client, ws client, query cache helpers, theme store, utils
```

Rules:
- Features import from `components/`, `lib/`, and other features' public `index.ts` only, never
  their internals (enforced by oxlint). Cross-cutting state that features and `app/` both need
  (e.g. the theme store) lives in `lib/`.
- Routes stay thin; logic lives in features.
- Generated code (`lib/api/`) is never edited by hand.

### 10.4 Design guidelines

All frontend work follows Martin's design guidelines
(`~/.agents/skills/martin-design-guidelines/SKILL.md`). Here's how they apply to this app:

- **Predictability**: standard chat-app layout and placement (sidebar left, composer bottom,
  settings bottom of sidebar). No novel navigation patterns.
- **Motion by frequency**:
  - No animation: keyboard-triggered actions (send on Enter, chat switching shortcuts, command
    palette), theme switch.
  - Minimal or none: new messages appearing, sidebar hover/selection, activity line updates
    (`shimmer` is the exception: it signals live work).
  - Standard: dialogs, sheets, popovers (mention autocomplete), toasts.
  - Delight allowed: first-run setup, creating your first agent.
- **Timing and easing**: shared easing and duration tokens as CSS variables (custom ease-out
  curves from easing.dev, never `ease-in`). Button feedback 100–160 ms, tooltips/popovers
  125–200 ms, dropdowns 150–250 ms, dialogs/sheets 200–300 ms.
- **Performance**: animate only `transform` and `opacity`; CSS for predetermined animations,
  WAAPI or springs for interruptible ones (preserve momentum on reversals).
- **Consistency**: one component per concept (one avatar, one bubble, one prompt card) reused
  everywhere; no one-off styles.
- **Trust**: approval prompts state exactly what will run and where; destructive or external
  actions use destructive/warning colors; turning on an agent's `trusted` mode and deleting
  things require confirmation; connector secrets are hidden by default.
- **Details**:
  - `scrollbar-gutter: stable` on `html`; `overscroll-behavior: contain` in dialogs and sheets.
  - Backdrop blur on a pseudo-layer, never on the dialog itself.
  - No stacked dialogs; navigate within one dialog with a way back.
  - Nested radii: outer radius = inner radius + padding (composer, prompt cards inside bubbles).
  - Composer grows with content (`field-sizing: content`) up to a max height.
  - `select-none` on sidebar items, buttons, and other frequently clicked controls.
  - Disabled actions explain themselves (e.g. a small shake when sending is blocked).
  - Grouped tooltips for icon toolbars (Base UI primitives make this easy; prefer the Base UI
    flavor of shadcn where it matters).
  - Metric-matched fallback fonts (`size-adjust`, ascent/descent overrides) for zero layout shift.
  - Reveal per-message actions (copy, reply) on hover or focus, not always visible.
- **Visual foundation** (tokens in `web/src/index.css`):
  - Three layers of depth instead of lines: the **frame** (window chrome; the sidebar sits on
    it), the **panel** (the main pane, inset in the frame with a 12px radius), and
    **surfaces** on the panel (bubbles, composer, cards). Borders are faint (6–8% of the text
    color).
  - Primary is neutral (light on dark, dark on light). `brand` is the one hue, kept for focus,
    links, unread counts and the "New" divider; `warning` marks things waiting on the user.
  - Pressable controls scale to 0.97 on press; hover feedback is instant.
- **Chat layout**:
  - The user's messages are bubbles on the right. An agent's plain replies are soft bubbles on
    the left; replies with code, tables or headings render flat at the column's width, with
    their actions in a footer row.
  - DMs show no avatar or name per message (the header says who it is). Groups head each run
    with the author's avatar and name, and keep a gutter so the run lines up.
  - Replies quote inside their own bubble. Prompts are standalone cards with a "Waiting for
    you" chip while pending.
  - What an agent is doing shows at the end of the transcript, where its reply will land:
    working avatar, shimmering label, elapsed time after 5s (`AgentActivity.since`). It
    appears after 120ms and lingers 500ms, so quick turns don't flicker.
  - The composer is one line until the text wraps, then grows a toolbar row; what it's
    attached to (a reply) sits on a slab tucked under its top edge.
  - Settings are a few real pages (General, Models & usage, Account; `?section=`), not one
    long scroll. The agent sheet is a header (model, status, Open chat, Computer) over tabs of
    settings grouped in cards.
  - Connectors is a card grid with the apps' own logos (`brand-logos.tsx`, Simple Icons):
    Yours and Discover, search, and Add for your own servers and webhooks. A card opens the
    add flow at that app.
  - The command palette groups Chats, Create, Go to and Preferences; agents' settings and
    computers appear once you type.
  - Sidebar rows: avatar with a status badge (working dots in DMs, amber dot when a question
    waits), name and time, preview, then "Waiting" or the unread count.

## 11. Auth and networking

- Single user. First run sets up the account, then onboarding: connect OpenCode Go (API key),
  then create the starter agent (name + model). The app is gated until onboarding completes.
- Environment variables only hold server settings (address, data dir, web dir, cookies, allowed
  origins). Everything else (provider key, models, agents) is configured in the app and stored in
  the database.
- Login: password (argon2id) + TOTP, and/or passkeys (WebAuthn). Sessions in HTTP-only,
  Secure, SameSite=Strict cookies; CSRF protection on mutating requests; login rate limiting.
- Bind modes: `tailscale` (listen only on the tailnet interface; default), `public` (behind TLS,
  via built-in ACME or a reverse proxy).
- Webhook path can be exposed independently (see §7.2).

## 12. Data model (SQLite)

```
users(id, username, password_hash, totp_secret, created_at)
settings(key, value, updated_at)                    -- secrets encrypted (see §9)
passkeys(id, user_id, credential, created_at)
sessions(id, user_id, expires_at, ...)

agents(id, name, instructions, personality, model, utility_model, language, notifications,
       trust_mode, is_admin, sandbox_id, created_at)
sandboxes(id, name, image, container_id, volume, limits_json, created_at)

chats(id, kind /* dm|group */, name, created_at)
chat_members(chat_id, agent_id, position)          -- user is implicit in every chat
messages(id, chat_id, author_kind /* user|agent|system */, author_agent_id,
         body, prompt_json, reply_to /* message id, same chat */, created_at)
message_mentions(message_id, mentioned /* user|agent:<id> */)
prompts(id, message_id, agent_id, kind, payload_json, status, answer_json, answered_at)
reads(chat_id, reader /* user|agent:<id> */, last_message_id)

events(id, agent_id, kind, payload_json, created_at, consumed_at)   -- the inbox
context_entries(id, agent_id, role, content_json, created_at, compacted_into)
context_summaries(id, agent_id, content, covers_until, created_at)

memories(id, agent_id, text, source_chat_id, created_at)
tasks(id, agent_id, name, purpose, type, spec_json, enabled, last_fired_at, created_at)

connector_accounts(id, type, name, credentials_enc, config_json, created_at)
grants(agent_id, account_id)
signals(id, account_id, type, payload_json, received_at)

push_subscriptions(id, endpoint, keys_json, created_at)
```

## 13. Repo layout

Monorepo: a Go module at the root plus a pnpm workspace (`pnpm-workspace.yaml` lists `web` and
`desktop`). Root scripts run lint, format, typecheck, tests, and codegen across both sides.

```
api/openapi.yaml      API contract (codegen source for Go and TS)
cmd/openbotd/            main binary
internal/
  api/                REST + WebSocket handlers
  auth/
  store/              SQLite access, migrations
  runtime/            agent loop, context assembly, compaction
  tools/              built-in tools
  group/              group coordinator
  scheduler/          cron / once
  connectors/         interface + slack/, linear/, github/, render/, mcp/
  sandbox/            Docker manager
  model/              OpenCode Go client
  push/               Web Push
web/                  React app (also the PWA), pnpm package
desktop/              Electron shell, pnpm package (loads web build)
sandbox-image/        Dockerfile for the default agent image
docs/
deploy/               docker-compose, example config
```

## 14. Milestones

1. **Skeleton** ✅: monorepo, `openbotd`, auth, WebSocket, chat UI, OpenCode Go client, starter agent.
2. **Agent runtime** ✅: inbox, single-loop turns, mid-turn injection, resume after restarts,
   memories, compaction.
3. **Sandboxes** ✅: Docker sandboxes, command and file tools, `/shared`, shared sandboxes.
4. **Managing agents from chat** ✅: onboarding intro, create/update/delete agents (deleting,
   from the agent's settings or by an admin agent after the user approves, removes its DM,
   memories, tasks, history, grants and unshared sandbox; its group messages stay without an
   author), clickable questions, approvals, trusted mode, reactions.
5. **Tasks** ✅: cron / one-off / signal tasks, time zones, Web Push notifications.
6. **Groups** ✅: group chats, turn coordinator, @mentions.
7. **Connectors** ✅: Webhook, GitHub, Linear, Slack, Render, MCP; grants; signal tasks.
8. **Packaging**: the installer, releases and Tailscale binding exist; Electron and PWA install
   polish remain.

Not yet built: TOTP / passkey login.

## 15. Open questions

- Per-model context window sizes: the models endpoint doesn't report them; keep a config table.
- Cost/rate limits of the subscription under many agents and group rounds — may need per-agent
  budgets or a global concurrency limit.
- Prompt injection via connector content (Slack messages, issue bodies) reaching trusted agents:
  mark connector content as untrusted in context; consider forcing approvals for external actions
  triggered by signals even in `trusted` mode (configurable).
- Should agents eventually be able to DM each other?
