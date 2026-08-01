#!/usr/bin/env bash
# Install the latest fcvm release and download Firecracker, jailer, and kernel.
# Usage:
#   curl -sSL https://raw.githubusercontent.com/pminnebach/fcvm/refs/heads/main/install.sh | sudo bash
set -euo pipefail

REPO="pminnebach/fcvm"
INSTALL_BIN="/usr/local/bin"
FCVM_BIN="${INSTALL_BIN}/fcvm"

die() {
  printf 'error: %s\n' "$*" >&2
  exit 1
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "required command not found: $1"
}

user_home() {
  if [[ -n "${SUDO_USER:-}" ]]; then
    getent passwd "${SUDO_USER}" | cut -d: -f6
    return
  fi
  printf '%s\n' "${HOME}"
}

run_install() {
  if [[ -w "${INSTALL_BIN}" ]]; then
    install -m 755 "$1" "$2"
  else
    need_cmd sudo
    sudo install -m 755 "$1" "$2"
  fi
}

status_of() {
  local path="$1"
  if [[ -e "${path}" ]]; then
    printf 'present'
  else
    printf 'missing'
  fi
}

cmd_path() {
  command -v "$1" 2>/dev/null || true
}

tool_path() {
  local p
  p="$(cmd_path "$1")"
  if [[ -n "${p}" ]]; then
    printf '%s\n' "${p}"
  else
    printf '(not found)\n'
  fi
}

tool_status() {
  if command -v "$1" >/dev/null 2>&1; then
    printf 'present\n'
  else
    printf 'missing\n'
  fi
}

normalize_ver() {
  local v="$1"
  v="${v#v}"
  printf '%s\n' "${v}"
}

installed_version() {
  local out
  if ! command -v fcvm >/dev/null 2>&1; then
    return 1
  fi
  out="$(fcvm version 2>/dev/null || true)"
  # "fcvm 1.2.3 (supported Firecracker …)"
  if [[ "${out}" =~ ^fcvm[[:space:]]+([^[:space:]]+) ]]; then
    printf '%s\n' "${BASH_REMATCH[1]}"
    return 0
  fi
  return 1
}

latest_tag() {
  local tag url
  # Prefer the API; on failure (rate limit, etc.) fall through to the redirect.
  tag="$(
    curl -sSL "https://api.github.com/repos/${REPO}/releases/latest" |
      sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' |
      head -n1 || true
  )"
  if [[ -n "${tag}" ]]; then
    printf '%s\n' "${tag}"
    return 0
  fi
  url="$(curl -fsSIL -o /dev/null -w '%{url_effective}' "https://github.com/${REPO}/releases/latest")"
  tag="${url##*/}"
  [[ -n "${tag}" && "${tag}" != "latest" ]] || die "could not resolve latest release tag"
  printf '%s\n' "${tag}"
}

verify_sha256() {
  local file="$1" expected="$2" actual
  actual="$(sha256sum "${file}" | awk '{print $1}')"
  [[ "${actual}" == "${expected}" ]] || die "checksum mismatch for ${file}: got ${actual}, want ${expected}"
}

checksum_for() {
  local sums_file="$1" archive_name="$2" line sum
  line="$(grep -E "[[:space:]]${archive_name}\$" "${sums_file}" | head -n1 || true)"
  [[ -n "${line}" ]] || die "checksum for ${archive_name} not found in ${sums_file}"
  sum="$(printf '%s\n' "${line}" | awk '{print $1}')"
  [[ -n "${sum}" ]] || die "empty checksum for ${archive_name}"
  printf '%s\n' "${sum}"
}

print_row() {
  printf '%-14s  %-42s  %s\n' "$1" "$2" "$3"
}

main() {
  need_cmd curl
  need_cmd tar
  need_cmd sha256sum
  need_cmd install
  need_cmd mktemp

  local os arch
  os="$(uname -s | tr '[:upper:]' '[:lower:]')"
  arch="$(uname -m)"
  [[ "${os}" == "linux" ]] || die "only linux is supported (got ${os})"
  case "${arch}" in
    x86_64 | amd64) ;;
    *) die "only amd64/x86_64 is supported (got ${arch})" ;;
  esac

  local tag ver home state
  tag="$(latest_tag)"
  ver="$(normalize_ver "${tag}")"
  home="$(user_home)"
  [[ -n "${home}" ]] || die "could not resolve home directory"
  state="${home}/.fcvm"

  printf 'latest release: %s\n' "${tag}"

  local current=""
  if current="$(installed_version)"; then
    if [[ "$(normalize_ver "${current}")" == "${ver}" ]]; then
      printf 'fcvm %s already installed, skipping binary download\n' "${current}"
    else
      current=""
    fi
  fi

  if [[ -z "${current}" ]]; then
    local tmp archive_url sums_url archive_name sums_name expected
    tmp="$(mktemp -d)"
    trap 'rm -rf "${tmp}"' EXIT

    archive_name="fcvm_${ver}_linux_amd64.tar.gz"
    sums_name="fcvm_${ver}_checksums.txt"
    archive_url="https://github.com/${REPO}/releases/download/${tag}/${archive_name}"
    sums_url="https://github.com/${REPO}/releases/download/${tag}/${sums_name}"

    printf 'downloading %s\n' "${archive_name}"
    curl -fsSL -o "${tmp}/${archive_name}" "${archive_url}"
    curl -fsSL -o "${tmp}/${sums_name}" "${sums_url}"

    expected="$(checksum_for "${tmp}/${sums_name}" "${archive_name}")"
    verify_sha256 "${tmp}/${archive_name}" "${expected}"

    tar -xzf "${tmp}/${archive_name}" -C "${tmp}"
    [[ -f "${tmp}/fcvm" ]] || die "archive did not contain fcvm binary"
    run_install "${tmp}/fcvm" "${FCVM_BIN}"
    printf 'installed fcvm %s to %s\n' "${ver}" "${FCVM_BIN}"

    rm -rf "${tmp}"
    trap - EXIT
  fi

  command -v fcvm >/dev/null 2>&1 || die "fcvm not found on PATH after install"

  local docker_bin firecracker_bin jailer_bin kernel_path rootfs_path
  docker_bin="$(cmd_path docker)"
  firecracker_bin="${state}/bin/firecracker"
  jailer_bin="${state}/bin/jailer"
  kernel_path="${state}/images/vmlinux"
  rootfs_path="${state}/images/rootfs.ext4"

  printf 'downloading firecracker and jailer\n'
  fcvm download firecracker

  if [[ -e "${kernel_path}" ]]; then
    printf 'kernel already present at %s, skipping\n' "${kernel_path}"
  else
    printf 'downloading kernel\n'
    fcvm download kernel
  fi

  printf '\n'
  print_row "Dependency" "Path" "Status"
  print_row "------------" "------------------------------------------" "------"
  print_row "fcvm" "${FCVM_BIN}" "$(status_of "${FCVM_BIN}")"
  print_row "firecracker" "${firecracker_bin}" "$(status_of "${firecracker_bin}")"
  print_row "jailer" "${jailer_bin}" "$(status_of "${jailer_bin}")"
  print_row "kernel" "${kernel_path}" "$(status_of "${kernel_path}")"
  print_row "rootfs" "${rootfs_path}" "$(status_of "${rootfs_path}")"
  if [[ -n "${docker_bin}" ]]; then
    print_row "docker" "${docker_bin}" "present"
  else
    print_row "docker" "(not found)" "missing"
  fi
  print_row "kvm" "/dev/kvm" "$(status_of /dev/kvm)"
  print_row "ip" "$(tool_path ip)" "$(tool_status ip)"
  print_row "iptables" "$(tool_path iptables)" "$(tool_status iptables)"
  print_row "mkfs.ext4" "$(tool_path mkfs.ext4)" "$(tool_status mkfs.ext4)"
  print_row "truncate" "$(tool_path truncate)" "$(tool_status truncate)"

  printf '\n'
  if [[ -z "${docker_bin}" ]]; then
    printf 'Docker is not installed. Install Docker to build a custom rootfs.\n'
  fi
  printf 'Build a custom rootfs with:\n'
  printf '  sudo fcvm build-rootfs --dockerfile ./Dockerfile\n'
}

main "$@"
