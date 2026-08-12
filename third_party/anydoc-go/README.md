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

Keep this list current: it is the diff a future upgrade has to re-apply. Items
2–4 are bugs in the upstream PR and are worth sending back to it.

1. `Cargo.toml` — depends on the published `anydoc = "=0.1.8"` crate instead of
   the workspace path dependency, declares its own empty `[workspace]`, and
   repeats the upstream release profile (`lto`, `strip`), which it would
   otherwise inherit from the anydoc workspace.
2. `anydoc.go` — every ABI call runs inside `call()`, which pins the goroutine
   with `runtime.LockOSThread` for the duration. The ABI reports the error
   message through a thread-local slot that a *second* call
   (`anydoc_last_error`) reads, and Go may resume a goroutine on a different OS
   thread once a cgo call returns: without pinning, a failed conversion reports
   an empty message, or one belonging to another document parsed on the thread
   it landed on. Reproduced at roughly 1 in 1500 concurrent conversions; the
   regression test is `TestErrorDetailSurvivesConcurrency` in
   `internal/infrastructure/docparser/anydoc`.
3. `model.go` — decoder preallocations are bounded by the bytes left in the
   buffer (`capFor`), and `need` rejects a negative length. A count taken
   straight from the buffer is only trustworthy while the Rust encoder and this
   decoder agree; on a skew, `make([]Block, 0, n)` would exhaust memory before
   the first bounds check, turning a version mismatch into a dead process.
4. `src/lib.rs` — the three conversion entry points run inside `guarded()`,
   which catches a panic and reports it as a malformed document. A panic
   escaping an `extern "C"` function aborts the process, and WeKnora parses
   untrusted uploads in the same process that serves the API. Note the limit:
   this cannot contain a stack overflow or an allocation failure, which is why
   the dependency pin below matters as much as the guard.
5. Removed the upstream CLI (`cmd/anydoc`) and the binding test suite, which
   reads fixtures from the anydoc repository. WeKnora's own tests live in
   `internal/infrastructure/docparser/anydoc`.

## Dependency pinning and audit

`Cargo.lock` is committed and `scripts/build-anydoc-lib.sh` builds with
`--locked`, so the archive is always the audited dependency tree. CI runs
`cargo audit` against it, because the crate that fails here is the one parsing
untrusted uploads inside the API process.

That matters concretely: with `lopdf` 0.41 — what the upstream PR's own
lockfile resolved to — a ~100 KB PDF holding a deeply nested catalog array
kills the process with a stack overflow (`RUSTSEC-2026-0187`), which neither
`guarded()` nor Go's `recover` can contain. anydoc 0.1.8 moved to `lopdf` 0.42
and the same input comes back as an ordinary error;
`TestDeeplyNestedPDFFailsWithoutKillingTheProcess` keeps it that way.

`cargo audit` currently reports one allowed warning: `ttf-parser` 0.25.1 is
unmaintained (`RUSTSEC-2026-0192`), pulled in transitively by the PDF stack. It
is not a vulnerability and nothing here can fix it, so warnings report without
failing the job.

## Known upstream limitation

Markdown rendering drops embedded images: `ImageSource::Asset` renders as its
alt text, and the bytes are only reachable through the document model, which
also means two parses for a document whose images are wanted. The renderer
(`document_to_markdown`) is private to the anydoc crate, so a caller cannot
rewrite asset images into links and render the result itself. Until upstream
exports the renderer or offers an asset-URL option, `AnydocReader` appends the
images at the end of the document, labelled with the alt text and section the
document model reports.

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
