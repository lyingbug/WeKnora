# WeKnora plugins

Two ways to add a plugin without editing `container.go`:

| Path | Language | Rebuild WeKnora? | How it loads |
| --- | --- | --- | --- |
| `../plugins.d/<id>/plugin.yaml` | JS (`runtime: js`) or any (`runtime: http`) | No | Host scans `WEKNORA_PLUGIN_DIR` |
| `websearch-echo/` | Go | Yes (blank import) | `WEKNORA_PLUGINS=websearch.echo` |
| `sdk-ts/websearch/` | TypeScript sidecar | No | HTTP + `plugin.yaml` |

In-tree engines stay in `internal/plugin/websearch` (bundle `base`).
Design: `docs/dev/plugin-architecture.md`.
