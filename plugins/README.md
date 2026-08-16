# WeKnora plugins

Out-of-tree (or extractable) plugins live here. They register a factory with
`plugin.Register` and implement a capability seam. The process host mounts
them from `config/plugin_profile.yaml` / `WEKNORA_PLUGINS` — you do not edit
`internal/container/container.go`.

| Path | Seam | How to enable |
| --- | --- | --- |
| `websearch-echo/` | `web_search` | `WEKNORA_PLUGINS=websearch.echo` |

In-tree engines still ship in `internal/plugin/websearch` (bundle `base`).
New community plugins: copy `websearch-echo`, register a unique factory id,
blank-import the package from `internal/plugin/boot` (Go compile-time catalog),
then enable it in the profile. Runtime RPC/WASM loading is a later phase.

Design: `docs/dev/plugin-architecture.md`.
