#!/usr/bin/env bash
#
# install-gvisor.sh - Install gVisor (runsc) from the official apt release channel.
#
# Based on: https://gvisor.dev/docs/user_guide/install/
#
# Usage:
#   sudo ./files/install-gvisor.sh
#   sudo ./files/install-gvisor.sh --no-verify

set -euo pipefail

GVISOR_APT_KEYRING="/usr/share/keyrings/gvisor-archive-keyring.gpg"
GVISOR_APT_LIST="/etc/apt/sources.list.d/gvisor.list"

SKIP_VERIFY=false

_log_ts() { date -Iseconds; }

log()  { printf '[install-gvisor] %s %s\n' "$(_log_ts)" "$*" >&2; }
die()  { printf '[install-gvisor] %s ERROR: %s\n' "$(_log_ts)" "$*" >&2; exit 1; }

usage() {
  cat <<'EOF'
Install gVisor (runsc) from the official apt release channel (latest release).

Usage:
  install-gvisor.sh [options]

Options:
  --no-verify   Skip runsc --version verification
  -h, --help    Show this help
EOF
}

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --no-verify) SKIP_VERIFY=true; shift ;;
      -h|--help)   usage; exit 0 ;;
      *) die "unknown option: $1 (try --help)" ;;
    esac
  done
}

require_root() {
  [[ "${EUID}" -eq 0 ]] || die "this script must be run as root (try: sudo $0)"
}

install_dependencies() {
  log "installing apt https dependencies"
  apt-get update
  apt-get install -y \
    apt-transport-https \
    ca-certificates \
    curl \
    gnupg
}

setup_apt_repository() {
  local arch

  arch="$(dpkg --print-architecture)"
  log "setting up gVisor apt repository (arch=${arch}, suite=release)"

  curl -fsSL https://gvisor.dev/archive.key | gpg --dearmor -o "${GVISOR_APT_KEYRING}"

  tee "${GVISOR_APT_LIST}" >/dev/null <<EOF
deb [arch=${arch} signed-by=${GVISOR_APT_KEYRING}] https://storage.googleapis.com/gvisor/releases release main
EOF

  apt-get update
}

install_runsc() {
  log "installing runsc (latest release)"
  apt-get install -y runsc
}

verify_installation() {
  log "verifying runsc"
  runsc --version
}

main() {
  parse_args "$@"
  require_root

  install_dependencies
  setup_apt_repository
  install_runsc

  if [[ "${SKIP_VERIFY}" != "true" ]]; then
    verify_installation
  else
    log "skipping runsc verification"
  fi

  log "gVisor installation complete"
}

main "$@"
