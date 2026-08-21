# TypeScript web search plugin

WeKnora cannot `import()` an npm package into the Go process. The
language-agnostic path is the same one MCP, LSP, and Dify local plugins
use: **the host launches your process and speaks JSON-RPC 2.0 on
stdin/stdout**. You implement handlers. You do not start an HTTP server.

## plugin.yaml

```yaml
id: websearch.brave-ts
name: Brave (TS)
seam: web_search
runtime: stdio
command: node
entry: index.js
provider: brave-ts
requires_api_key: true
```

Drop that folder in `WEKNORA_PLUGIN_DIR` (default `plugins.d/<id>/`).

## Author surface

```ts
import { serve } from "./serve.ts"

serve({
  async search(req) {
    return { results: [/* title, url, snippet */] }
  },
})
```

`npx --yes tsx plugins/sdk-ts/websearch/example.ts` is a one-file echo.
A copy that needs no tsx lives at `plugins.d/websearch-node-echo/`.

## Wire format

One JSON-RPC 2.0 object per line. Logs go to **stderr** (stdout is the
protocol). Methods:

| Method | Kind | Payload |
| --- | --- | --- |
| `websearch.search` | request | `{ query, max_results, include_date, parameters }` |
| `shutdown` | notification | none |

`parameters` is the tenant bag (`api_key`, `engine_id`, `base_url`, …).

## What about HTTP?

`runtime: http` is a **fallback** for a search API that already exists
as a remote service. Do not invent a sidecar just to write a plugin.
If you later need a shared multi-tenant endpoint, the JSON body is the
same `SearchRequest` / `SearchResponse` as `plugin.invoke` result.
`example-server.ts` stays only as that fallback illustration.
