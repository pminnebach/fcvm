#!/usr/bin/env bash
#
# install-docker.sh - Install Docker Engine on Ubuntu from Docker's apt repository.
#
# Based on: https://docs.docker.com/engine/install/ubuntu/
#
# Usage:
#   sudo ./scripts/install-docker.sh
#   sudo ./scripts/install-docker.sh --version '5:29.6.1-1~ubuntu.24.04~noble'
#   sudo ./scripts/install-docker.sh --user philip
#   sudo ./scripts/install-docker.sh --skip-uninstall --no-verify

set -euo pipefail

# shellcheck disable=SC2310,SC2311,SC2312

DOCKER_APT_KEY="/etc/apt/keyrings/docker.asc"
DOCKER_APT_SOURCES="/etc/apt/sources.list.d/docker.sources"
DOCKER_PACKAGES=(
  docker-ce
  docker-ce-cli
  containerd.io
  docker-buildx-plugin
  docker-compose-plugin
)
CONFLICTING_PACKAGES=(
  docker.io
  docker-compose
  docker-compose-v2
  docker-doc
  podman-docker
  containerd
  runc
)

VERSION_STRING=""
DOCKER_USER=""
SKIP_UNINSTALL=false
SKIP_VERIFY=false

_log_ts() { date -Iseconds; }

log()  { printf '[install-docker] %s %s\n' "$(_log_ts)" "$*" >&2; }
die()  { printf '[install-docker] %s ERROR: %s\n' "$(_log_ts)" "$*" >&2; exit 1; }

usage() {
  cat <<'EOF'
Install Docker Engine on Ubuntu from Docker's official apt repository.

Usage:
  install-docker.sh [options]

Options:
  --version VERSION   Install a specific docker-ce version (see: apt list --all-versions docker-ce)
  --user NAME         Add NAME to the docker group (non-root docker access)
  --skip-uninstall    Do not remove conflicting distro packages first
  --no-verify         Skip the hello-world verification container
  -h, --help          Show this help
EOF
}

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --version)
        [[ $# -ge 2 ]] || die "--version requires an argument"
        VERSION_STRING="$2"
        shift 2
        ;;
      --user)
        [[ $# -ge 2 ]] || die "--user requires an argument"
        DOCKER_USER="$2"
        shift 2
        ;;
      --skip-uninstall) SKIP_UNINSTALL=true; shift ;;
      --no-verify)      SKIP_VERIFY=true; shift ;;
      -h|--help)        usage; exit 0 ;;
      *) die "unknown option: $1 (try --help)" ;;
    esac
  done
}

require_root() {
  [[ "${EUID}" -eq 0 ]] || die "this script must be run as root (try: sudo $0)"
}

require_ubuntu() {
  [[ -f /etc/os-release ]] || die "/etc/os-release not found"
  # shellcheck disable=SC1091
  source /etc/os-release
  [[ "${ID:-}" == "ubuntu" ]] || die "this script supports Ubuntu only (detected: ${ID:-unknown})"
  log "detected Ubuntu ${VERSION_ID:-unknown} (${UBUNTU_CODENAME:-${VERSION_CODENAME:-unknown}})"
}

ubuntu_codename() {
  # shellcheck disable=SC1091
  . /etc/os-release
  printf '%s' "${UBUNTU_CODENAME:-${VERSION_CODENAME:-}}"
}

remove_conflicting_packages() {
  local installed=()
  local pkg

  for pkg in "${CONFLICTING_PACKAGES[@]}"; do
    if dpkg-query -W -f='${Status}' "${pkg}" 2>/dev/null | grep -q 'install ok installed'; then
      installed+=("${pkg}")
    fi
  done

  if [[ "${#installed[@]}" -eq 0 ]]; then
    log "no conflicting packages installed"
    return
  fi

  log "removing conflicting packages: ${installed[*]}"
  apt-get remove -y "${installed[@]}"
}

setup_apt_repository() {
  local codename

  codename="$(ubuntu_codename)"
  [[ -n "${codename}" ]] || die "could not determine Ubuntu codename"

  log "setting up Docker apt repository (suite=${codename})"
  apt-get update
  apt-get install -y ca-certificates curl
  install -m 0755 -d /etc/apt/keyrings
  curl -fsSL https://download.docker.com/linux/ubuntu/gpg -o "${DOCKER_APT_KEY}"
  chmod a+r "${DOCKER_APT_KEY}"

  tee "${DOCKER_APT_SOURCES}" >/dev/null <<EOF
Types: deb
URIs: https://download.docker.com/linux/ubuntu
Suites: ${codename}
Components: stable
Architectures: $(dpkg --print-architecture)
Signed-By: ${DOCKER_APT_KEY}
EOF

  apt-get update
}

install_docker_packages() {
  if [[ -n "${VERSION_STRING}" ]]; then
    log "installing Docker packages (version=${VERSION_STRING})"
    apt-get install -y \
      docker-ce="${VERSION_STRING}" \
      docker-ce-cli="${VERSION_STRING}" \
      containerd.io \
      docker-buildx-plugin \
      docker-compose-plugin
  else
    log "installing latest Docker packages"
    apt-get install -y "${DOCKER_PACKAGES[@]}"
  fi
}

ensure_docker_running() {
  if systemctl is-active --quiet docker; then
    log "docker service is already running"
  else
    log "starting docker service"
    systemctl start docker
  fi
  systemctl enable docker >/dev/null 2>&1 || true
}

verify_installation() {
  log "running hello-world container"
  docker run --rm hello-world
}

add_user_to_docker_group() {
  local user="${DOCKER_USER}"

  if [[ -z "${user}" && -n "${SUDO_USER:-}" && "${SUDO_USER}" != "root" ]]; then
    user="${SUDO_USER}"
  fi

  if [[ -z "${user}" ]]; then
    return
  fi

  if ! id "${user}" >/dev/null 2>&1; then
    die "user not found: ${user}"
  fi

  if id -nG "${user}" | tr ' ' '\n' | grep -qx docker; then
    log "user ${user} is already in the docker group"
    return
  fi

  log "adding ${user} to the docker group"
  usermod -aG docker "${user}"
  log "log out and back in (or run: newgrp docker) for group membership to take effect"
}

main() {
  parse_args "$@"
  require_root
  require_ubuntu

  if [[ "${SKIP_UNINSTALL}" != "true" ]]; then
    remove_conflicting_packages
  else
    log "skipping removal of conflicting packages"
  fi

  setup_apt_repository
  install_docker_packages
  ensure_docker_running

  if [[ "${SKIP_VERIFY}" != "true" ]]; then
    verify_installation
  else
    log "skipping hello-world verification"
  fi

  add_user_to_docker_group
  log "Docker Engine installation complete"
}

main "$@"
