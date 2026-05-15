# Agent Integration Guide for WeKnora Development

> This file provides operational guidance for AI coding agents working on the
> WeKnora codebase. For CLI agent-integration (invoking the `weknora` binary),
> see `cli/AGENTS.md`.

## Cursor Cloud specific instructions

### Architecture Overview

WeKnora is a multi-service application:

| Service | Stack | Dev Port |
|---------|-------|----------|
| Backend (API) | Go 1.26 + Gin | 8080 |
| Frontend (UI) | Vue 3 + Vite + TypeScript + TDesign | 5173 |
| DocReader | Python gRPC (uv-managed) | 50051 |
| PostgreSQL | ParadeDB v0.22.2-pg17 (pgvector) | 5432 |
| Redis | redis:7.0-alpine | 6379 |

### Starting the dev environment

The recommended workflow is documented in the root `README.md` under "Developer Guide":

```
make dev-start      # Docker: postgres + redis + docreader
make dev-app        # Go backend on :8080 (loads .env, overrides hosts to localhost)
make dev-frontend   # Vite dev server on :5173
```

#### Non-obvious caveats

1. **`libsqlite3-dev` is required** — the Go build uses CGO bindings for SQLite
   (sqlite-vec). Without the system package the build fails with
   `fatal error: sqlite3.h: No such file or directory`.

2. **Docker must be running** before `make dev-start`. The dev compose file
   (`docker-compose.dev.yml`) starts postgres, redis, and docreader. The
   docreader image is built from `docker/Dockerfile.docreader` on first run.

3. **`.env` must exist** — copy from `.env.example`. The dev script (`scripts/dev.sh`)
   sources `.env` and then overrides `DB_HOST`, `DOCREADER_ADDR`, `REDIS_ADDR`, etc.
   to `localhost` for local-backend access to the Docker containers.

4. **CGO flags** — always set `CGO_CFLAGS="-Wno-deprecated-declarations -Wno-gnu-folding-constant"`
   when building the Go backend to suppress warnings from vendored C code (DuckDB, sqlite-vec).

5. **golangci-lint** — As of May 2026, the latest golangci-lint (v2.12) is built with
   Go 1.25 and rejects Go 1.26 projects. Use `go vet ./...` as the primary lint check
   until golangci-lint releases a Go 1.26-compatible build.

6. **Frontend type-check** — `npx vue-tsc --build --noEmit` reports pre-existing TS
   errors in the repo (not blocking for development or `npm run dev`).

7. **Frontend uses npm** — the lockfile is `package-lock.json`; run `npm install`
   in `frontend/`.

### Running tests

```bash
# Go tests (handler package has tests)
CGO_CFLAGS="-Wno-deprecated-declarations -Wno-gnu-folding-constant" go test ./internal/handler/...

# Frontend (no test framework configured as of v0.5.1)
cd frontend && npx vue-tsc --build --noEmit   # type-check only
```

### Building

```bash
# Dev build
CGO_CFLAGS="-Wno-deprecated-declarations -Wno-gnu-folding-constant" go build -o WeKnora ./cmd/server

# Lite build (SQLite, no external deps)
make build-lite
```

### API authentication

- Register: `POST /api/v1/auth/register` with `{"username","password","email"}`
- Login: `POST /api/v1/auth/login` with `{"email","password"}` → returns `token`
- Use: `Authorization: Bearer <token>` header on all other API calls
