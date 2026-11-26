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
# Windows needs a different toolchain, because the archive is MSVC-built: it
# references MSVC's C++ RTTI and Winsock/ntdll imports that mingw's runtime has
# no equivalent for, so the default mingw gcc cannot link it. Set
#
#   CC=clang -target x86_64-pc-windows-msvc -fuse-ld=lld
#
# (clang auto-detects the installed Visual Studio for headers and libraries;
# -fuse-ld=lld is required, not preferred — Go probes the external linker and,
# unless it answers LLD, passes a gcc linker script that link.exe cannot read.)
# The emitted CGO_LDFLAGS name the Windows system libraries the Rust staticlib
# leaves to its consumer; see the windows case below.
#
# Deliberately POSIX-ish bash 3.2 compatible (no associative arrays): macOS
# ships bash 3.2 as /bin/bash, and that is what a `#!/usr/bin/env bash` script
# gets on the macOS CI runners.
#
# Usage: scripts/relate-ffi.sh <rust-target-triple> <dest-dir>

set -euo pipefail

RELATE_VERSION="1.3.0"

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
        aarch64-apple-darwin) echo "597c22706981c221dd3a47470710f6d6a0ee548693207402f1965f06621c4576" ;;
        aarch64-unknown-linux-gnu) echo "0d9980b9ab09c8db5fa9f4637058242976b93037ef2371c962f302e5b5ba0702" ;;
        x86_64-apple-darwin) echo "ca0ed3482b2be6f121438540ca3c7a5da32878288b19d7b070442a2f5f673e37" ;;
        x86_64-pc-windows-msvc) echo "a087ad5ea6f2426c79f9b679d7d511959f61443c1105c44ba39cb528044af972" ;;
        x86_64-unknown-linux-gnu) echo "d49574d9f072846a4cfbe81b4ed74f0d4cab7d4c0bf0af65d453ba92060e3b31" ;;
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

# Windows hands us a native path (D:\a\_temp). Bash and curl take it fine, but
# MSYS2 tar treats the backslashes as ESCAPES and reports
# "tar: D\:\a\\_temp/...: Cannot open: No such file or directory"; gcc would
# mangle them the same way. cygpath -m gives the mixed form (D:/a/_temp/...),
# which every Windows tool accepts and which has no backslashes to escape.
# Absent on linux/macOS, where dest_out is just dest.
if command -v cygpath >/dev/null 2>&1; then
    dest_out="$(cygpath -m "$dest")"
else
    dest_out="$dest"
fi

# The archive goes in on stdin rather than as `-f <path>`: GNU tar reads a
# leading `host:` in an archive argument as a REMOTE archive, and a Windows
# `$RUNNER_TEMP` (D:\a\_temp) matches that — "tar (child): Cannot connect to
# D: resolve failed". `--force-local` would fix it but bsdtar (macOS) has no
# such option; stdin is portable.
tar -xzf - -C "$dest_out" < "$archive"

# Static linking needs the archive ALONE in a directory: with the shared
# library next to it, the linker picks the shared one and the binary gains a
# runtime dependency instead.
mkdir -p "$dest/static"
case "$triple" in
    *-apple-darwin)
        # The Rust staticlib pulls iana_time_zone, which needs CoreFoundation;
        # the dylib carries it internally, the archive does not.
        cp "$dest/librelate_ffi.a" "$dest/static/"
        ldflags="-L$dest_out/static -lrelate_ffi -framework CoreFoundation"
        ;;
    *-pc-windows-msvc)
        # Windows builds this with clang in MSVC mode, NOT mingw: the archive is
        # MSVC-built and references MSVC's C++ RTTI (`??_7type_info@@6B@`) and
        # Winsock/ntdll imports that mingw's runtime has no equivalent for, so a
        # mingw link cannot succeed no matter which -l flags are added. clang
        # resolves `-lrelate_ffi` to `relate_ffi.lib` (MSVC has no `lib` prefix),
        # which also satisfies the Go client's own #cgo `-lrelate_ffi`
        # directives; the name is left exactly as the release ships it.
        cp "$dest/relate_ffi.lib" "$dest/static/"
        # A Rust `staticlib` deliberately omits the platform's native system
        # libraries — rustc expects the C consumer to supply them — and nothing
        # in a Go build knows Rust's list, so they are named here. Without them
        # lld-link stops at `undefined symbol: getpeername` / `WSASocketW` /
        # `getaddrinfo` (ws2_32), `NtReadFile` (ntdll) and so on. These are
        # import libraries for DLLs that ship with Windows itself, so the
        # resulting binary is still self-contained.
        ldflags="-L$dest_out/static \
-ladvapi32 -lbcrypt -lcomdlg32 -lcrypt32 -ldbghelp -ldnsapi -lgdi32 \
-liphlpapi -lkernel32 -lmsvcprt -lmswsock -lnetapi32 -lntdll -loldnames \
-lole32 -loleaut32 -lpowrprof -lpsapi -lschannel -lsecur32 -lshell32 \
-luser32 -luserenv -luuid -lwinmm -lwinspool -lws2_32 -lwsock32"
        ;;
    *)
        # -lm: the archive calls into libm (pow/exp/log/sin/…). macOS folds
        # libm into libSystem, glibc does not, so this is linux-only.
        cp "$dest/librelate_ffi.a" "$dest/static/"
        ldflags="-L$dest_out/static -lrelate_ffi -lm"
        ;;
esac

echo "relate-ffi: unpacked $asset into $dest (static link ready)" >&2
printf 'export CGO_CFLAGS="-I%s/include"\n' "$dest_out"
printf 'export CGO_LDFLAGS="%s"\n' "$ldflags"
