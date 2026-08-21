# WeKnora plugins

Add a capability without editing `container.go`:

| Path | Language | Rebuild WeKnora? | How it loads |
| --- | --- | --- | --- |
| `../plugins.d/<id>/plugin.yaml` | any (`runtime: stdio`) or JS (`runtime: js`) | No | Host scans `WEKNORA_PLUGIN_DIR` |
| `websearch-echo/` | Go | Yes (blank import) | `WEKNORA_PLUGINS=websearch.echo` |
| `sdk-ts/websearch/` | TypeScript | No | `serve()` on stdin/stdout |

`runtime: http` is a fallback for an already-running remote service, not
the way to write a new plugin.

In-tree engines stay in `internal/plugin/websearch` (bundle `base`).
Design: `docs/dev/plugin-architecture.md`.
