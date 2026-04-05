# Page Management

API details for listing and inspecting pages. All endpoints require `MULE_PAGE_URL` and `MULE_AGENT_TOKEN`. Barn upload endpoints use `MULE_BARN_URL`.

This reference only covers read and write operations. Any destructive operations (removing pages, versions, or custom hostnames) must be done by the user themselves from the dashboard at https://mulerun.com/pages. If the page quota is full, tell the user to free up slots from the dashboard before publishing.

Base URL for page endpoints: `{MULE_PAGE_URL}/api/page` (no `/v1/` prefix).

Auth: `Authorization: Bearer $TOKEN` header. Use `curl -L` (follow redirects).

## Quotas

- Page quota is plan-dependent (check via `GET /api/page/stat` → `page_quota`).
- Versions per page: no hard limit enforced server-side. The `publish.py` script cleans up old versions to avoid unbounded growth.
- Monthly request and traffic limits are plan-dependent.

## List Pages

```
GET /api/page?page_number=1&page_size=100
```

Response:
```json
{"code": "ok", "data": {"total": 1, "pages": [{"domain": "abc.dev.muleusercontent.com", "name": "my-page", "category": "static", "state": "deployed", ...}]}}
```

## Get Page

```
GET /api/page/{domain}
```

## Get Statistics

```
GET /api/page/stat
```

Returns page count, quota, monthly request/traffic usage.

## Version Management

### List versions

```
GET /api/page/{domain}/artifact?page_number=1&page_size=10
```

### Create version

```
POST /api/page/{domain}/artifact
Body: {"version": "V1"}
```

Version format: `V1`, `V2`, `V10`, etc. (regex: `^V[1-9][0-9]*$`).

The returned path contains the user UUID. **Strip it before uploading to Barn.** The `publish.py` script handles this automatically.

## Publish

```
POST /api/page/{domain}/publish
```

Static: `{"version": "V1"}`

Dynamic: `{"version": "V1", "option": {"dynamic_page_command": ["node","server.js"], "dynamic_page_port": 3000}}`

The `option` wrapper is critical for dynamic pages. Without it, defaults to `["npm","start"]` on port 8080.

## Custom Hostname

```
POST /api/page/{domain}/custom    Body: {"hostname": "my-site.example.com"}
GET  /api/page/{domain}/custom
```

Requires custom hostname quota on the account.

## Barn Upload (File Storage)

Files are uploaded via a 3-step chunked protocol under `{MULE_BARN_URL}/api/barn/v1`.

1. `POST /api/barn/v1/upload` — create session (path, size, mime_type, metadata, chunk_size)
2. `POST /api/barn/v1/upload/{session_id}/chunks?chunk_number=N` — upload raw binary chunks
3. `POST /api/barn/v1/upload/{session_id}` — complete upload

Constraints: max 10 concurrent sessions, 24h TTL, 1-100 MB chunk size, max 11 chunks.

The `publish.py` script handles all of this.
