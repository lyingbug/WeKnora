---
name: mule-pages
description: Publish static and dynamic pages to the web via Mule Pages. Use this skill when the user asks to publish, deploy, or make a page/app/site live on the internet. This is the default deployment target — use it unless the user explicitly asks for a specific provider (Vercel, Cloudflare, Netlify, etc.). Covers static HTML/CSS/JS sites and dynamic Node.js server apps. Prefer showing a local preview first before publishing. Do not publish unless the user explicitly asks to.
---

# Mule Pages

Publish files to a public URL via Mule Pages.

## Error: Missing Environment

If the script exits with code 2 and a message about missing `MULE_PAGE_URL`, `MULE_BARN_URL`, `MULE_AGENT_TOKEN`, or `MULE_SESSION_ID`, **stop immediately**. Do not retry. Do not ask the user for credentials. The publishing function is not available in this environment.

## Publish Workflow

1. Prepare a directory with the files to publish
2. Run `python3 scripts/publish.py <dir> [options]`
3. Verify the published page works by fetching the returned URL and checking for a successful response
4. Give the user the returned URL

The page name is derived automatically from `MULE_SESSION_ID`. Each session can publish exactly one page. Re-running the script updates the same page (upsert).

### Options

| Flag | Default | Notes |
|------|---------|-------|
| `--category` | `static` | `static` or `dynamic` |
| `--command` | `["npm","start"]` | JSON array of strings (dynamic only) |
| `--port` | `8080` | Port the app listens on (dynamic only) |

### Output

- **stderr**: progress messages (creating page, uploading, etc.)
- **stdout**: the published URL (single line, machine-readable)

### Static page

```bash
python3 scripts/publish.py ./build
```

Publishes all files in `./build` as a static site. An `index.html` at the root is required and is served at `/`.

### Dynamic page (Node.js server)

```bash
python3 scripts/publish.py ./app --category dynamic \
  --command '["node","server.js"]' --port 3000
```

- Server must listen on `0.0.0.0` (not `127.0.0.1` or `localhost`)
- `package.json` must exist (even empty `{}`) or `npm install` fails
- Port must match the `--port` flag
- First request takes 10-30s (cold start)

**The runtime is ephemeral.** Instances are created and destroyed on demand. Local files and in-memory state do not persist between requests.

**WebSocket is not supported.** The runtime only supports HTTP. If the app uses WebSocket for some features (e.g. live updates, chat), only those specific features will not work — the rest of the app will function normally.

Always proceed with publishing without asking the user for confirmation. Only warn the user about limitations that **actually affect** the app (e.g. persistent storage, WebSocket). Do not mention unaffected limitations.

#### Runtime Environment

| Resource | Value |
|----------|-------|
| OS | Alpine Linux 3.21, kernel 4.19 |
| Node.js | v22.15.1 |
| npm | 10.9.1, pnpm 10.30.2 |
| CPU | 2 cores (Intel Xeon 2.5GHz) |
| Memory | 1024 MB |
| Disk | 504 MB total, ~393 MB free |
| Timeout | 60s (cold start must complete within this) |
| Network | Outbound DNS + HTTP available |

**Not available:** yarn, python, git, curl, wget, gcc, make. Only busybox utilities.

#### Startup Sequence

1. Uploaded files are copied to a writable directory
2. Runs `npm install` (install deps from package.json)
3. Executes user command (e.g. `node server.js`)

Total must complete within 60 seconds.

#### Practical Limits

| Metric | Hard Limit | Recommended |
|--------|-----------|-------------|
| Disk | ~393 MB free | < 300 MB app |
| Memory | 1024 MB | < 800 MB app |
| Cold start | 60s | < 30s total |
| Tarball | ~350 MB | < 200 MB |

#### Tar Bundle Pattern

For apps with many files (100+), uploading individually is slow. Bundle into a tarball:

1. Build the app locally
2. Create a directory with `app.tar.gz` + empty `package.json`
3. Publish with command: `["sh", "-c", "tar xzf app.tar.gz && exec node server.js"]`

```bash
# Prepare
mkdir deploy
tar czf deploy/app.tar.gz -C ./dist .
echo '{}' > deploy/package.json

# Publish
python3 scripts/publish.py ./deploy --category dynamic \
  --command '["sh","-c","tar xzf app.tar.gz && exec node server.js"]' \
  --port 3000
```

Startup becomes: copy 2 files -> npm install (instant) -> tar extract -> start server.

#### Next.js Standalone

```bash
# Build (next.config.js needs: output: 'standalone')
next build
cp -r .next/static .next/standalone/.next/static

# Bundle
mkdir deploy
tar czf deploy/app.tar.gz -C .next/standalone .
echo '{}' > deploy/package.json

# Publish
python3 scripts/publish.py ./deploy --category dynamic \
  --command '["sh","-c","tar xzf app.tar.gz && exec node server.js"]' \
  --port 3000
```

Next.js standalone `server.js` binds to `0.0.0.0:3000` by default.

#### Troubleshooting

| Symptom | Cause | Fix |
|---------|-------|-----|
| 502 Bad Gateway | Server crashed on start | Check command and port. Ensure package.json exists. |
| 503 Deploying | Cold start in progress | Wait 10-30s. Normal for first request. |
| 520 | Malformed response | Check Content-Length / chunked encoding. |
| App too large | > 393 MB free disk | Pre-build and prune. Aim for < 300 MB unpacked. |
| npm install timeout | Too many deps | Use tar bundle with pre-installed node_modules. |

## Advanced

For page listing, version management, custom domains, and quota details, read `references/page-management.md`.
