#!/usr/bin/env bash
# Copyright (c) "Neo4j"
# Neo4j Sweden AB [http://neo4j.com]
#
# Fetch the pinned Relate FFI native library for one Rust target triple and
# print the cgo environment a `-tags relate` build needs.
#
#   eval "$(scripts/relate-ffi.sh aarch64-apple-darwin /tmp/relate-ffi)"
#   CGO_ENABLED=1 go build -tags relate ./cmd/neo4j-cli
#
# The link is STATIC: the script leaves librelate_ffi.a alone in <dest>/static
# (a linker prefers a shared library when both sit in one directory) so the
# resulting binary is self-contained — nothing to ship beside it, no rpath and
# no separate signing/notarization step. The shared library is unpacked as
# well, for anyone who wants to link dynamically instead.
#
# Every download is verified against the sha256 GitHub reports for that release
# asset, so a tampered or truncated archive fails closed instead of silently
# linking into the binary.
#
# Deliberately POSIX-ish bash 3.2 compatible (no associative arrays): macOS
# ships bash 3.2 as /bin/bash, and that is what a `#!/usr/bin/env bash` script
# gets on the macOS CI runners.
#
# Usage: scripts/relate-ffi.sh <rust-target-triple> <dest-dir>

set -euo pipefail

RELATE_VERSION="1.0.0"

fail() {
    echo "relate-ffi: $*" >&2
    exit 1
}

usage() {
    echo "usage: $0 <rust-target-triple> <dest-dir>" >&2
    echo "known triples:" >&2
    echo "  aarch64-apple-darwin" >&2
    echo "  aarch64-unknown-linux-gnu" >&2
    echo "  x86_64-apple-darwin" >&2
    echo "  x86_64-pc-windows-msvc" >&2
    echo "  x86_64-unknown-linux-gnu" >&2
}

# Hashes stdin, not a named file: GNU coreutils prefixes a backslash to its
# output line when the FILENAME needs escaping, and a Windows `$RUNNER_TEMP`
# (D:\a\_temp) contains backslashes — `cut -d' ' -f1` then yields "\<hash>"
# and every checksum on that platform mismatches. With no filename there is
# nothing to escape.
sha256_of() {
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum | cut -d' ' -f1
    elif command -v shasum >/dev/null 2>&1; then
        shasum -a 256 | cut -d' ' -f1
    else
        fail "neither sha256sum nor shasum is available to verify the archive"
    fi
}

# sha256 of relate-ffi-<triple>.tar.gz in the relatedb/relate v<RELATE_VERSION>
# release. A triple with no entry is refused rather than downloaded unchecked;
# bump these together with RELATE_VERSION.
expected_sha() {
    case "$1" in
        aarch64-apple-darwin) echo "f300b85ad91e5d2ed4973f070ffa0eaad8a8dbf92d9362217f190a949578194f" ;;
        aarch64-unknown-linux-gnu) echo "654f0d2e4694b407e3702a8afb2da0b9d898207906ea328508d208226a7a465f" ;;
        x86_64-apple-darwin) echo "3357cbacfd82763e4d2e22025d284c6e6328a444ad670c3c900230d2160f9dbf" ;;
        x86_64-pc-windows-msvc) echo "f5ce7aff1d086d05634c6f02844586625e514e1d35f7e6cc5bc144f1bd7239cb" ;;
        x86_64-unknown-linux-gnu) echo "bff57f9f030d45618b8909d4c4fb4f3379027ba002250e61e0407a3c03158387" ;;
        *) echo "" ;;
    esac
}

[[ $# -eq 2 ]] || { usage; exit 2; }

triple="$1"
dest="$2"

want_sha="$(expected_sha "$triple")"
[[ -n "$want_sha" ]] || { echo "relate-ffi: unknown target triple '$triple'" >&2; usage; exit 2; }

asset="relate-ffi-${triple}.tar.gz"
url="https://github.com/relatedb/relate/releases/download/v${RELATE_VERSION}/${asset}"

mkdir -p "$dest"
archive="$dest/$asset"

if [[ ! -f "$archive" ]]; then
    echo "relate-ffi: downloading $asset (v$RELATE_VERSION)" >&2
    curl -fsSL --retry 3 --retry-delay 2 -o "$archive" "$url" ||
        fail "download failed: $url"
fi

got_sha="$(sha256_of < "$archive")"
if [[ "$got_sha" != "$want_sha" ]]; then
    rm -f "$archive"
    fail "sha256 mismatch for $asset: got $got_sha, want $want_sha (removed the file; re-run to refetch)"
fi

tar -xzf "$archive" -C "$dest"

# Static linking needs the archive ALONE in a directory: with the shared
# library next to it, the linker picks the shared one and the binary gains a
# runtime dependency instead.
mkdir -p "$dest/static"
case "$triple" in
    *-apple-darwin)
        # The Rust staticlib pulls iana_time_zone, which needs CoreFoundation;
        # the dylib carries it internally, the archive does not.
        cp "$dest/librelate_ffi.a" "$dest/static/"
        ldflags="-L$dest/static -lrelate_ffi -framework CoreFoundation"
        ;;
    *-pc-windows-msvc)
        # The MSVC staticlib keeps its MSVC name, which mingw's `-lrelate_ffi`
        # does not find (it searches lib*.a / lib*.dll.a), so name the file.
        cp "$dest/relate_ffi.lib" "$dest/static/"
        ldflags="$dest/static/relate_ffi.lib"
        ;;
    *)
        # -lm: the archive calls into libm (pow/exp/log/sin/…). macOS folds
        # libm into libSystem, glibc does not, so this is linux-only.
        cp "$dest/librelate_ffi.a" "$dest/static/"
        ldflags="-L$dest/static -lrelate_ffi -lm"
        ;;
esac

echo "relate-ffi: unpacked $asset into $dest (static link ready)" >&2
printf 'export CGO_CFLAGS="-I%s/include"\n' "$dest"
printf 'export CGO_LDFLAGS="%s"\n' "$ldflags"
