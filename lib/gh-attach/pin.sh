# shellcheck shell=bash
# lib/gh-attach/pin.sh — the pin of the one gh-image build, read as DATA. Sourced by bin/gh-attach and
# setup/install/install-gh-image.sh so the installer and every run judge the checksum with one copy of the code.

# ghimg_pin_file <root> — the pin: always <root>/setup/install/gh-image.pin, <root> being the checkout the script
# runs from. NOT overridable from the environment: whoever sets a variable must not choose the hash the binary is
# judged by (the suite runs a copy of the scripts under a sandbox root instead).
ghimg_pin_file() { printf '%s/setup/install/gh-image.pin' "$1"; }

# ghimg_pin_get <pin> <key> [<subkey>] — the value of `key` (or of `key subkey`), never sourced, first match.
ghimg_pin_get() {
  awk -v k="$2" -v s="${3:-}" '
    /^[[:space:]]*#/ || NF == 0 { next }
    s == "" && $1 == k { print $2; exit }
    s != "" && $1 == k && $2 == s { print $3; exit }' "$1"
}

# ghimg_platform — <os>-<arch> as the release names its binaries (darwin-arm64, linux-amd64, …).
ghimg_platform() {
  local os arch
  os="$(uname -s | tr 'A-Z' 'a-z')"; arch="$(uname -m)"
  case "$arch" in x86_64) arch=amd64 ;; aarch64|arm64) arch=arm64 ;; esac
  printf '%s-%s' "$os" "$arch"
}

# ghimg_sha256 <file> — the hex digest, or nothing.
ghimg_sha256() {
  if command -v shasum >/dev/null 2>&1; then shasum -a 256 "$1" 2>/dev/null | awk '{print $1}'
  elif command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" 2>/dev/null | awk '{print $1}'
  fi
}

# ghimg_dir <version> — where the pinned build lives: a tree hw owns, never ~/.claude* or ~/.pi. A fixed path under
# $HOME (no override): what is found there is still judged by the pin's sha256, so HOME cannot bless a binary.
ghimg_dir() { printf '%s/.local/share/hw/tools/gh-image/%s' "$HOME" "$1"; }
