# TypeScript web search plugin

WeKnora cannot `import()` a npm package into the Go process. The TS-shaped
workflow is: write a small HTTP service, drop a `plugin.yaml` that points at
it, restart. No fork, no blank import, no rebuild.

## Protocol

`POST` JSON to `endpoint`:

```ts
type SearchRequest = {
  query: string
  max_results: number
  include_date: boolean
  parameters: {
    api_key?: string
    engine_id?: string
    base_url?: string
    extra_config?: Record<string, string>
  }
}

type SearchResponse = {
  results: Array<{
    title: string
    url: string
    snippet?: string
    content?: string
    source?: string
  }>
  error?: string
}
```

## plugin.yaml

```yaml
id: websearch.brave-ts
name: Brave (TS sidecar)
seam: web_search
runtime: http
endpoint: http://127.0.0.1:9101/search
provider: brave-ts
requires_api_key: true
```

Put that file in `WEKNORA_PLUGIN_DIR` (default `plugins.d/<id>/`). Add
`127.0.0.1` to `SSRF_WHITELIST_EXTRA` only if the sidecar itself needs to
be fetched through WeKnora's outbound client; the plugin endpoint in
`plugin.yaml` is operator-trusted and may be loopback.

## Example

`example-server.ts` is a one-file echo sidecar. Run with any Node 18+:

```bash
npx --yes tsx plugins/sdk-ts/websearch/example-server.ts
```
