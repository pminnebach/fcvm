#!/usr/bin/env bash
#
# install-kasm.sh - Install Kasm Workspaces (single-server, rolling release).
#
# Based on: https://docs.kasm.com/docs/tutorials/install/single-server-install/
#
# Usage:
#   sudo ./install-kasm.sh
#   sudo ./install-kasm.sh --admin-password 'secret' --user-password 'secret'
#   sudo ./install-kasm.sh --proxy-port 8443
#   sudo ./install-kasm.sh --activation-key-file /path/to/activation_key.txt
#
# Environment:
#   KASM_VERSION   Release series (default: 1.19.0)
#   KASM_WORK_DIR  Download/extract directory (default: /tmp)

set -euo pipefail

# shellcheck disable=SC2310,SC2311,SC2312

KASM_BASE_URL="https://kasm-static-content.s3.amazonaws.com"
KASM_VERSION="${KASM_VERSION:-1.19.0}"
KASM_WORK_DIR="${KASM_WORK_DIR:-/tmp}"

ADMIN_PASSWORD=""
USER_PASSWORD=""
PROXY_PORT=""
ACTIVATION_KEY_FILE=""
SKIP_DOCKER_CHECK=false
USE_STATIC_IMAGES=false
INSTALL_ARGS=()

_log_ts() { date -Iseconds; }

log() { printf '[install-kasm] %s %s\n' "$(_log_ts)" "$*" >&2; }
die() { printf '[install-kasm] %s ERROR: %s\n' "$(_log_ts)" "$*" >&2; exit 1; }

# #region agent log
_debug_log() {
  local hypothesis_id="$1" message="$2" data="$3"
  local ts _debug_path _debug_dir
  ts=$(date +%s%3N 2>/dev/null || date +%s)
  _debug_path="/tmp/debug-ab6709.log"
  _debug_dir="$(dirname "${_debug_path}")"
  mkdir -p "${_debug_dir}" 2>/dev/null || true
  { printf '{"sessionId":"ab6709","timestamp":%s,"location":"install-kasm.sh","message":"%s","data":%s,"hypothesisId":"%s","runId":"%s"}\n' \
    "${ts}" "${message}" "${data}" "${hypothesis_id}" "${DEBUG_RUN_ID:-pre-fix}"; } \
    >> "${_debug_path}" 2>/dev/null || true
}
# #endregion

usage() {
  cat <<'EOF'
Install Kasm Workspaces on a single server using the rolling release bundle.

Usage:
  install-kasm.sh [options] [-- <extra install.sh flags>]

Options:
  --admin-password PASS       Set the default admin password (admin@kasm.local)
  --user-password PASS        Set the default user password (user@kasm.local)
  --proxy-port PORT           HTTPS listening port (default: 443)
  --activation-key-file PATH  License activation key file
  --use-static-images         Use static versioned images instead of rolling
  --skip-docker-check         Do not verify Docker is installed and running
  -h, --help                  Show this help

Any arguments after "--" are passed directly to kasm_release/install.sh
(e.g. -W for default workspace images, -D to skip starting services).

Environment:
  KASM_VERSION    Release series (default: 1.19.0)
  KASM_WORK_DIR   Working directory for download/extract (default: /tmp)
EOF
}

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --admin-password)
        [[ $# -ge 2 ]] || die "--admin-password requires an argument"
        ADMIN_PASSWORD="$2"
        shift 2
        ;;
      --user-password)
        [[ $# -ge 2 ]] || die "--user-password requires an argument"
        USER_PASSWORD="$2"
        shift 2
        ;;
      --proxy-port)
        [[ $# -ge 2 ]] || die "--proxy-port requires an argument"
        PROXY_PORT="$2"
        shift 2
        ;;
      --activation-key-file)
        [[ $# -ge 2 ]] || die "--activation-key-file requires an argument"
        ACTIVATION_KEY_FILE="$2"
        shift 2
        ;;
      --use-static-images) USE_STATIC_IMAGES=true; shift ;;
      --skip-docker-check) SKIP_DOCKER_CHECK=true; shift ;;
      -h|--help)           usage; exit 0 ;;
      --)
        shift
        INSTALL_ARGS+=("$@")
        break
        ;;
      *) die "unknown option: $1 (try --help)" ;;
    esac
  done
}

require_root() {
  [[ "${EUID}" -eq 0 ]] || die "this script must be run as root (try: sudo $0)"
}

require_docker() {
  command -v docker >/dev/null 2>&1 \
    || die "docker not found; install Docker first (see install-docker.sh)"

  if ! docker info >/dev/null 2>&1; then
    if systemctl is-active --quiet docker 2>/dev/null; then
      die "docker is installed but not accessible; check permissions or socket"
    fi
    log "starting docker service"
    systemctl start docker
  fi

  log "docker is available ($(docker --version))"
}

release_tarball() {
  printf 'kasm_release_%s-latest.tar.gz' "${KASM_VERSION}"
}

release_checksum() {
  printf '%s.sha256sum' "$(release_tarball)"
}

download_release() {
  local tarball checksum url

  tarball="$(release_tarball)"
  checksum="$(release_checksum)"
  url="${KASM_BASE_URL}/${tarball}"

  mkdir -p "${KASM_WORK_DIR}"
  cd "${KASM_WORK_DIR}"

  log "downloading rolling release ${KASM_VERSION} to ${KASM_WORK_DIR}"
  curl --fail-early -fsSLO "${url}" -fsSLO "${KASM_BASE_URL}/${checksum}"

  log "verifying ${tarball} checksum"
  sha256sum --check "${checksum}"

  log "extracting ${tarball}"
  tar -xf "${tarball}"
  [[ -f kasm_release/install.sh ]] || die "extracted bundle missing kasm_release/install.sh"
}

ensure_install_deps() {
  local sudo_path modprobe_path modules_exists

  sudo_path="$(command -v sudo || true)"
  modprobe_path="$(command -v modprobe || true)"
  [[ -f /etc/modules ]] && modules_exists=true || modules_exists=false

  # #region agent log
  _debug_log "A" "host deps before ensure" \
    "$(printf '{"euid":%s,"sudoPath":"%s","modprobePath":"%s","modulesFile":%s}' \
      "${EUID}" "${sudo_path}" "${modprobe_path}" "${modules_exists}")"
  # #endregion

  if [[ -z "${sudo_path}" ]]; then
    log "installing sudo shim (kasm installer requires sudo on PATH even as root)"
    install -d /usr/local/bin
    printf '#!/bin/sh\nexec "$@"\n' > /usr/local/bin/sudo
    chmod 755 /usr/local/bin/sudo
    sudo_path="$(command -v sudo || true)"
  fi

  if [[ -z "${modprobe_path}" ]] && command -v apt-get >/dev/null 2>&1; then
    log "installing kmod (optional; wireguard/v4l2 deps may still fail in microvm kernels)"
    apt-get update -qq
    DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends kmod \
      || log "kmod install failed; continuing (--ignore-dep-failures)"
    modprobe_path="$(command -v modprobe || true)"
  fi

  [[ -f /etc/modules ]] || touch /etc/modules
  [[ -f /etc/modules ]] && modules_exists=true || modules_exists=false

  # #region agent log
  _debug_log "A" "host deps after ensure" \
    "$(printf '{"euid":%s,"sudoPath":"%s","modprobePath":"%s","modulesFile":%s}' \
      "${EUID}" "${sudo_path}" "${modprobe_path}" "${modules_exists}")"
  # #endregion

  command -v sudo >/dev/null 2>&1 \
    || die "sudo is required by kasm_release/install.sh but could not be installed"
}

ensure_tun_device() {
  local tun_exists=false tun_char=false
  [[ -e /dev/net/tun ]] && tun_exists=true
  [[ -c /dev/net/tun ]] && tun_char=true

  # #region agent log
  _debug_log "F" "tun device before ensure" \
    "$(printf '{"tunExists":%s,"tunChar":%s}' "${tun_exists}" "${tun_char}")"
  # #endregion

  if command -v modprobe >/dev/null 2>&1; then
    modprobe tun 2>/dev/null || true
  fi

  if [[ ! -c /dev/net/tun ]]; then
    log "creating /dev/net/tun for kasm sidecar network plugin"
    mkdir -p /dev/net
    mknod /dev/net/tun c 10 200 2>/dev/null || true
    chmod 666 /dev/net/tun 2>/dev/null || true
  fi

  tun_exists=false
  tun_char=false
  [[ -e /dev/net/tun ]] && tun_exists=true
  [[ -c /dev/net/tun ]] && tun_char=true

  # #region agent log
  _debug_log "F" "tun device after ensure" \
    "$(printf '{"tunExists":%s,"tunChar":%s}' "${tun_exists}" "${tun_char}")"
  # #endregion

  [[ -c /dev/net/tun ]]
}

run_installer() {
  ensure_install_deps
  local skip_egress=false
  if ! ensure_tun_device; then
    log "no /dev/net/tun available; skipping kasm egress/sidecar network plugin"
    skip_egress=true
  fi

  local -a cmd=(bash kasm_release/install.sh -e --ignore-dep-failures -J 1024)
  if [[ "${skip_egress}" == "true" ]]; then
    cmd+=(--skip-egress)
  fi

  cd "${KASM_WORK_DIR}"

  if [[ "${USE_STATIC_IMAGES}" == "true" ]]; then
    cmd+=(-f)
  fi

  [[ -n "${ADMIN_PASSWORD}" ]] && cmd+=(-P "${ADMIN_PASSWORD}")
  [[ -n "${USER_PASSWORD}" ]]  && cmd+=(-U "${USER_PASSWORD}")
  [[ -n "${PROXY_PORT}" ]]     && cmd+=(-L "${PROXY_PORT}")

  if [[ -n "${ACTIVATION_KEY_FILE}" ]]; then
    [[ -f "${ACTIVATION_KEY_FILE}" ]] \
      || die "activation key file not found: ${ACTIVATION_KEY_FILE}"
    cmd+=(-a "${ACTIVATION_KEY_FILE}")
  fi

  if [[ "${#INSTALL_ARGS[@]}" -gt 0 ]]; then
    cmd+=("${INSTALL_ARGS[@]}")
  fi

  log "running kasm installer (rolling release)"

  # #region agent log
  _debug_log "G" "installer command" \
    "$(printf '{"cmd":"%s","skipEgress":%s}' "${cmd[*]}" "${skip_egress}")"
  # #endregion

  local exit_code=0
  set +e
  "${cmd[@]}"
  exit_code=$?
  set -e

  if (( exit_code != 0 )); then
    # #region agent log
    _debug_log "G" "installer failed" "$(printf '{"exitCode":%s}' "${exit_code}")"
    # #endregion
    die "kasm installer exited with status ${exit_code}"
  fi

  # #region agent log
  _debug_log "G" "installer succeeded" "$(printf '{"exitCode":%s}' "${exit_code}")"
  # #endregion
}

main() {
  parse_args "$@"
  require_root

  if [[ "${SKIP_DOCKER_CHECK}" != "true" ]]; then
    require_docker
  else
    log "skipping docker check"
  fi

  download_release
  run_installer
  log "Kasm Workspaces installation complete — open https://<host>:${PROXY_PORT:-443}"
}

main "$@"
