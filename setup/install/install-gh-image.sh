#!/usr/bin/env bash
# Build the pinned gh-image from the audited SOURCE and install it for bin/gh-attach (idempotent).
#
#   setup/install/install-gh-image.sh            fetch the pin's commit from github.com/drogers0/gh-image, build, verify, install
#   setup/install/install-gh-image.sh --from D   the same from D, a local git checkout whose HEAD is the pin's commit
#
# WHAT IT HOLDS. setup/install/gh-image.pin names the source commit, the Go toolchain and the sha256 of the build
# per platform. The source is the committed tree of THAT commit (fetched by full sha: git hashes every object it
# receives; a checkout whose HEAD is not the pin's commit is refused before anything is built), the modules are the
# ones go.sum of that commit lists (-mod=readonly: a missing or different hash is a build failure), the build runs
# in an environment of its own (no GOFLAGS, no go env file, no cgo, no VCS stamping, -trimpath), and the resulting
# binary is hashed BEFORE it is placed: a hash that is not the pin's leaves nothing installed and exits 1. Installed to
# ~/.local/share/hw/tools/gh-image/<version>/gh-image (a fixed path, no override) — a tree the toolchain owns,
# never ~/.claude* or ~/.pi. bin/gh-attach hashes it again on EVERY run.
# NETWORK: one https git fetch of the pin's commit from github.com/drogers0/gh-image, and the Go module proxy
# (proxy.golang.org, verified by go.sum and sum.golang.org) for the modules; nothing is uploaded. No gh-image code runs.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# shellcheck source=../../lib/gh-attach/pin.sh
. "$ROOT/lib/gh-attach/pin.sh"

die() { printf 'install-gh-image: %s\n' "$*" >&2; exit 1; }
from=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    --from) [ "$#" -ge 2 ] || die "--from needs a directory"; from="$2"; shift 2 ;;
    -h|--help) sed -n '2,15p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) die "unknown argument: $1" ;;
  esac
done

pin="$(ghimg_pin_file "$ROOT")"
[ -r "$pin" ] || die "no pin at $pin"
version="$(ghimg_pin_get "$pin" version)"
commit="$(ghimg_pin_get "$pin" commit)"
source_url="$(ghimg_pin_get "$pin" source)"
want_go="$(ghimg_pin_get "$pin" go)"
platform="$(ghimg_platform)"
want="$(ghimg_pin_get "$pin" sha256 "$platform")"
[ -n "$version" ] || die "the pin names no version"
[[ "$commit" =~ ^[0-9a-f]{40}$ ]] || die "the pin names no 40-hex commit"
[ -n "$want_go" ] || die "the pin names no Go toolchain"
[ -n "$want" ] || die "the pin has no sha256 for $platform: build it on that platform, audit nothing new, and record its hash"

GIT=/usr/bin/git; [ -x "$GIT" ] || GIT="$(command -v git || true)"; [ -n "$GIT" ] || die "git is needed"
GOBIN_PATH="$(command -v go || true)"; [ -n "$GOBIN_PATH" ] || die "go is needed to build gh-image from source"
got_go="$("$GOBIN_PATH" env GOVERSION 2>/dev/null || true)"
[ "$got_go" = "$want_go" ] || die "the pin builds with $want_go and this machine has '${got_go:-no go}': install that toolchain (the build's hash depends on it)"

work="$(mktemp -d)"
trap 'chmod -R u+w "$work" 2>/dev/null; rm -rf "$work"' EXIT   # the module cache is read-only
# git and go each run in an environment of their own: nothing the caller exported (GOFLAGS, GOPROXY, GIT_*, a go env file,
# an insteadOf rewrite) chooses the source, the modules or the build flags.
genv() { env -i HOME="$work/home" PATH="/usr/bin:/bin" GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_SYSTEM=/dev/null GIT_TERMINAL_PROMPT=0 "$@"; }
mkdir -p "$work/home" "$work/src"

if [ -n "$from" ]; then
  [ -d "$from" ] || die "not a directory: $from"
  head="$(genv "$GIT" -C "$from" rev-parse HEAD 2>/dev/null)" || die "not a git checkout: $from"
  [ "$head" = "$commit" ] || die "REFUSED: $from is at commit $head, the pin says $commit. Nothing was built or installed."
  repo="$from"; ref="$commit"
else
  [ "$source_url" = "https://github.com/drogers0/gh-image" ] \
    || die "refusing a source that is not https://github.com/drogers0/gh-image: '$source_url'"
  mkdir -p "$work/fetch"
  genv "$GIT" -C "$work/fetch" init -q
  genv "$GIT" -C "$work/fetch" -c protocol.allow=never -c protocol.https.allow=always fetch -q --depth 1 "$source_url.git" "$commit" \
    || die "could not fetch $commit from $source_url"
  got_commit="$(genv "$GIT" -C "$work/fetch" rev-parse 'FETCH_HEAD^{commit}')"
  [ "$got_commit" = "$commit" ] || die "REFUSED: the fetch answered $got_commit, the pin says $commit. Nothing was built or installed."
  repo="$work/fetch"; ref="$commit"
fi
# the committed tree of the pinned commit, and nothing else (an untracked or modified file in a checkout is not in it)
genv "$GIT" -C "$repo" archive --format=tar "$ref" | tar -x -C "$work/src" || die "could not export $commit"
[ -f "$work/src/go.mod" ] && [ -f "$work/src/go.sum" ] || die "the commit has no go.mod and go.sum: modules could not be checked"

( cd "$work/src" && env -i HOME="$work/home" PATH="/usr/bin:/bin:$(dirname "$GOBIN_PATH")" \
    GOENV=off GOTOOLCHAIN=local GOFLAGS=-mod=readonly CGO_ENABLED=0 GOPROXY=https://proxy.golang.org GOSUMDB=sum.golang.org \
    GOMODCACHE="$work/gomod" GOCACHE="$work/gocache" GOPATH="$work/gopath" \
    "$GOBIN_PATH" build -trimpath -buildvcs=false -ldflags "-s -w -X main.version=${version#v}" -o "$work/gh-image" . ) \
  || die "the build failed (go.sum or the toolchain refused). Nothing was installed."

got="$(ghimg_sha256 "$work/gh-image")"
[ -n "$got" ] || die "no shasum or sha256sum to hash with"
if [ "$got" != "$want" ]; then
  die "REFUSED: sha256 mismatch for the build of $commit on $platform ($version, $want_go): pin $want, got $got. Nothing was installed."
fi

dest="$(ghimg_dir "$version")"
mkdir -p "$dest"
install -m 0755 "$work/gh-image" "$dest/gh-image"
printf 'installed gh-image %s (%s), built from commit %s with %s, sha256 %s -> %s/gh-image\n' "$version" "$platform" "$commit" "$want_go" "$got" "$dest"
