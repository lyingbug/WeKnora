#!/usr/bin/env bash
# Build the anydoc static archive that the `anydoc` build tag links.
#
# The archive is a Rust build artifact (~30 MB), so it is neither committed nor
# downloaded: this script builds it from third_party/anydoc-go, which pins the
# published anydoc crate, and drops it where cgo looks for it.
#
# Usage:
#   scripts/build-anydoc-lib.sh              # host platform
#   TARGET=aarch64-unknown-linux-musl scripts/build-anydoc-lib.sh
set -euo pipefail

repo_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
crate_dir="$repo_root/third_party/anydoc-go"

if ! command -v cargo >/dev/null 2>&1; then
  echo "error: cargo not found. Install Rust (https://rustup.rs) to build the anydoc archive." >&2
  exit 1
fi

# The Rust target triple decides the archive; the directory name mirrors what
# the cgo LDFLAGS in third_party/anydoc-go expect for that platform.
target=${TARGET:-$(rustc -vV | sed -n 's/^host: //p')}
case "$target" in
  x86_64-apple-darwin) lib_dir=darwin_amd64 ;;
  aarch64-apple-darwin) lib_dir=darwin_arm64 ;;
  x86_64-pc-windows-msvc) lib_dir=windows_amd64 ;;
  x86_64-unknown-linux-gnu) lib_dir=linux_amd64_gnu ;;
  aarch64-unknown-linux-gnu) lib_dir=linux_arm64_gnu ;;
  x86_64-unknown-linux-musl) lib_dir=linux_amd64_musl ;;
  aarch64-unknown-linux-musl) lib_dir=linux_arm64_musl ;;
  *)
    echo "error: unsupported target '$target'." >&2
    echo "Supported: {x86_64,aarch64}-{apple-darwin,unknown-linux-{gnu,musl}}, x86_64-pc-windows-msvc" >&2
    exit 1
    ;;
esac

case "$target" in
  *windows-msvc) lib_name=anydoc_go.lib ;;
  *) lib_name=libanydoc_go.a ;;
esac

# --locked: build exactly the dependency versions in the committed Cargo.lock.
# That lockfile is what pins the patched pdf-inspector/lopdf, so a silent
# resolver drift must fail the build rather than ship an unaudited tree.
echo "Building anydoc archive for $target"
cargo build --release --locked --manifest-path "$crate_dir/Cargo.toml" --target "$target"

dest="$crate_dir/lib/$lib_dir"
mkdir -p "$dest"
cp "$crate_dir/target/$target/release/$lib_name" "$dest/$lib_name"

echo "Wrote $dest/$lib_name"
echo "Build WeKnora with the archive: go build -tags anydoc ./cmd/server"
