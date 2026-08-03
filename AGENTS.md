# AGENTS.md

## Cursor Cloud specific instructions

WeKnora is a polyglot monorepo (see `README.md`). For local development the product runs as:

- Backend API (Go, `./cmd/server`) on `:8080` — run with `make dev-app` (runs on the host, needs Postgres + Redis).
- Web UI (Vue 3 + Vite, `frontend/`) on `:5173`, proxies `/api` to `:8080` — run with `make dev-frontend`.
- Postgres (paradedb) + Redis — the required infra, started via Docker: `docker compose -f docker-compose.dev.yml up -d postgres redis`.
- DocReader (`:50051`) — OPTIONAL; only needed to parse uploaded files. Its image is heavy (built from `docker/Dockerfile.docreader`); not needed for register/login/KB-creation/text-entry.
- Ollama (`:11434`) — local LLM provider (no API key). Needed for embeddings/chat; creating a knowledge base requires an embedding model.

Standard build/lint/test commands are documented in the `Makefile`, `README.md` (Developer Guide), and `docs/开发指南.md`. Frontend scripts are in `frontend/package.json` (`npm run dev|test|type-check`).

### Non-obvious caveats (read before starting services)

- The environment is snapshot-based; the update script only refreshes dependencies. The long-running services (Docker daemon, Postgres/Redis, backend, frontend, Ollama) are NOT auto-started — start them as below.
- Docker daemon: this VM has no systemd. Start it with `sudo dockerd &` (or check `docker info` first). The daemon config at `/etc/docker/daemon.json` is required and already set to `storage-driver: fuse-overlayfs` with `features.containerd-snapshotter: false` (needed for Docker 29 on this kernel); iptables is set to legacy. Make the socket usable without sudo: `sudo chmod 666 /var/run/docker.sock` (the `ubuntu` user is in the `docker` group, effective in a fresh shell).
- `.env` must exist (`cp .env.example .env`); it is gitignored. `docker-compose*.yml` require it.
- **Use the Postgres stack, not Lite mode.** `make run-lite` (SQLite) is broken against the current code: `migrations/sqlite/` is a stale schema snapshot (missing e.g. the `tenants.api_principal_config` column and the `system_settings` / `task_pending_ops` tables), so registration/workspace creation fails. The Postgres migrations (`migrations/versioned/`, 79 files, applied automatically on backend start) are complete and current.
- `make dev-app` maps container hostnames to `localhost` (DB_HOST, REDIS_ADDR, DOCREADER_ADDR, …) but does NOT override `OLLAMA_BASE_URL`. For the host-run backend set `OLLAMA_BASE_URL=http://localhost:11434` in `.env` (the `.env.example` default `host.docker.internal` only resolves inside containers).
- Model base URLs pass through SSRF validation which blocks loopback by default. To register Ollama models (or any localhost endpoint) add `SSRF_WHITELIST=127.0.0.1,localhost` to `.env` and restart the backend.
- Ollama has no systemd here: start with `OLLAMA_HOST=0.0.0.0:11434 ollama serve &`. A working local setup: `ollama pull nomic-embed-text` (embedding, dimension 768) and `ollama pull qwen2.5:0.5b` (chat). In the UI, create a knowledge base by selecting `nomic-embed-text` as the embedding model and `qwen2.5:0.5b` as the LLM.
- The backend runs without Air (not installed) → plain `go run ./cmd/server`. There is no hot reload; restart the `make dev-app` process after Go changes.
- Redis is password-protected (`REDIS_PASSWORD` in `.env`); `redis-cli` needs `-a "$REDIS_PASSWORD"`.
- CGO is required (sqlite-vec / duckdb bindings). The system needs `libsqlite3-dev`; build/test with `CGO_ENABLED=1 CGO_CFLAGS="-Wno-deprecated-declarations"`.
- `golangci-lint` must be built with the repo's Go toolchain (go 1.26) or it refuses to run. Install with `GOTOOLCHAIN=go1.26.0 go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.6.2`. Config is `.golangci.yml` (v2). Scope lint to changed packages (repo-wide has pre-existing findings).
- Testing gotcha specific to the agent harness: a literal email address written in a tool call (shell/curl body, or an instruction to the computer-use agent) is redacted to `[email protected]`, which fails the app's `email` validation and looks like a false "registration bug". Assemble the email from parts at runtime (e.g. build `local@domain` from separate shell variables) or, in the UI, type the local-part, then `@`, then the domain into the field separately.
