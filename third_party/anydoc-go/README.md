# anydoc-go (vendored)

Go bindings for [anydoc](https://github.com/firecrawl/anydoc), the Rust library
that converts Word, PowerPoint, Excel, OpenDocument, RTF, EPUB, CSV, and PDF
documents to GitHub-Flavored Markdown. WeKnora links them through cgo so office
documents can be parsed inside the Go process, without the Python docreader
service.

## Why this is vendored

The bindings come from the open pull request
[firecrawl/anydoc#30](https://github.com/firecrawl/anydoc/pull/30). Until it
merges and upstream tags `go/vX.Y.Z`, there is no module to `go get`: the PR
also expects a maintainer-only workflow to commit per-platform archives that do
not exist yet. So the binding source lives here and `go.mod` points at it:

```
replace github.com/firecrawl/anydoc/go => ./third_party/anydoc-go
```

When the upstream module is published, delete this directory, drop the
`replace`, and require the published version. Nothing else in WeKnora changes:
only `internal/infrastructure/docparser/anydoc/backend_cgo.go` imports it.

## Provenance

| | |
| --- | --- |
| Upstream PR | firecrawl/anydoc#30 ("feat: add Go bindings") |
| PR head | `1a7a6c0` |
| Rebased onto | `4e3089b` (`chore: release v0.1.8`) |
| anydoc crate | `0.1.8` (from crates.io, pinned with `=`) |
| License | MIT (see LICENSE) |

The PR branched before anydoc 0.1.7, so it was rebased onto v0.1.8 before
vendoring. The rebase conflicts were all release bookkeeping — the workspace
member list, the README binding sections, and the version-agreement script,
which the wasm bindings had touched in the meantime — plus the binding's own
version, bumped from 0.1.3 to 0.1.8. Taking v0.1.8 also picks up the
`pdf-inspector` 0.1.8 bump that fixes [RUSTSEC-2026-0187](https://rustsec.org/advisories/RUSTSEC-2026-0187.html),
the `lopdf` stack overflow that aborts the process on a hostile PDF.

## Local modifications

Keep this list current: it is the diff a future upgrade has to re-apply.

1. `Cargo.toml` — depends on the published `anydoc = "=0.1.8"` crate instead of
   the workspace path dependency, declares its own empty `[workspace]`, and
   repeats the upstream release profile (`lto`, `strip`), which it would
   otherwise inherit from the anydoc workspace.
2. `src/lib.rs` — the three conversion entry points run inside `guarded()`,
   which catches a panic and reports it as a malformed document. A panic
   escaping an `extern "C"` function aborts the process, and WeKnora parses
   untrusted uploads in the same process that serves the API.
3. Removed the upstream CLI (`cmd/anydoc`) and the binding test suite, which
   reads fixtures from the anydoc repository. WeKnora's own tests live in
   `internal/infrastructure/docparser/anydoc`.

## Building the archive

cgo links `lib/<platform>/libanydoc_go.a`, which is a build artifact (~30 MB)
and is therefore git-ignored rather than committed:

```bash
scripts/build-anydoc-lib.sh                          # host platform
TARGET=aarch64-unknown-linux-musl scripts/build-anydoc-lib.sh
```

Then build WeKnora with the engine linked in:

```bash
make build-anydoc          # or: go build -tags anydoc ./cmd/server
```

Builds without the `anydoc` tag need no Rust toolchain and no archive; the
engine simply reports itself as unavailable.
