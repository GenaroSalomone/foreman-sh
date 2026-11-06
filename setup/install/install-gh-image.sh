#!/usr/bin/env bash
# Install the pinned gh-image build that bin/gh-attach runs (idempotent).
#
#   setup/install/install-gh-image.sh            download the pinned release binary, verify, install
#   setup/install/install-gh-image.sh --from F   verify the local file F against the pin and install it
#
# WHAT IT HOLDS. The version and the sha256 per platform come from setup/install/gh-image.pin. The binary is
# hashed BEFORE it is placed: a checksum that is not the pin's leaves nothing installed and exits 1. Installed to
# ~/.local/share/hw/tools/gh-image/<version>/gh-image (a fixed path, no override) — a tree the toolchain owns,
# never ~/.claude* or ~/.pi. bin/gh-attach hashes it again on EVERY run.
# NETWORK: one https GET to the pin's url-base (github.com/drogers0/gh-image releases); nothing is uploaded.
set -euo pipefail
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
# shellcheck source=../../lib/gh-attach/pin.sh
. "$ROOT/lib/gh-attach/pin.sh"

die() { printf 'install-gh-image: %s\n' "$*" >&2; exit 1; }
from=""
while [ "$#" -gt 0 ]; do
  case "$1" in
    --from) [ "$#" -ge 2 ] || die "--from needs a file"; from="$2"; shift 2 ;;
    -h|--help) sed -n '2,12p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) die "unknown argument: $1" ;;
  esac
done

pin="$(ghimg_pin_file "$ROOT")"
[ -r "$pin" ] || die "no pin at $pin"
version="$(ghimg_pin_get "$pin" version)"
base="$(ghimg_pin_get "$pin" url-base)"
platform="$(ghimg_platform)"
want="$(ghimg_pin_get "$pin" sha256 "$platform")"
[ -n "$version" ] || die "the pin names no version"
[ -n "$want" ] || die "the pin has no sha256 for $platform"

work="$(mktemp -d)"; trap 'rm -rf "$work"' EXIT
if [ -n "$from" ]; then
  [ -f "$from" ] || die "not a file: $from"
  cp "$from" "$work/gh-image"
else
  case "$base" in
    https://github.com/drogers0/gh-image/releases/download/*) ;;
    *) die "refusing a url-base that is not github.com/drogers0/gh-image releases: $base" ;;
  esac
  curl -fsSL --proto '=https' --max-time 120 -o "$work/gh-image" "$base/$platform" \
    || die "download failed: $base/$platform"
fi

got="$(ghimg_sha256 "$work/gh-image")"
[ -n "$got" ] || die "no shasum or sha256sum to hash with"
if [ "$got" != "$want" ]; then
  die "REFUSED: sha256 mismatch for $platform ($version): pin $want, got $got. Nothing was installed."
fi

dest="$(ghimg_dir "$version")"
mkdir -p "$dest"
install -m 0755 "$work/gh-image" "$dest/gh-image"
printf 'installed gh-image %s (%s) sha256 %s -> %s/gh-image\n' "$version" "$platform" "$got" "$dest"
