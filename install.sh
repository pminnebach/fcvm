#!/usr/bin/env bash
# Install the latest fcvm release and download Firecracker, jailer, and kernel.
# Usage:
#   curl -sSL https://raw.githubusercontent.com/pminnebach/fcvm/refs/heads/main/install.sh | sudo bash
set -euo pipefail

REPO="pminnebach/fcvm"
INSTALL_BIN="/usr/local/bin"
FCVM_BIN="${INSTALL_BIN}/fcvm"

RED=$'\033[31m'
ORANGE=$'\033[38;5;208m'
GREEN=$'\033[32m'
BLUE=$'\033[34m'
RESET=$'\033[0m'

# usage: log <level> <color_esc> <message...>
log() {
  local level="$1" color="$2"
  shift 2
  local ts
  ts="$(date '+%Y-%m-%d %H:%M:%S')"
  if [[ -t 2 ]] && [[ ! -v NO_COLOR ]]; then
    printf '%s | %s%s%s | %s\n' "${ts}" "${color}" "${level}" "${RESET}" "$*" >&2
  else
    printf '%s | %s | %s\n' "${ts}" "${level}" "$*" >&2
  fi
}

log_error() { log error "${RED}" "$*"; }
log_warn() { log warn "${ORANGE}" "$*"; }
log_info() { log info "${GREEN}" "$*"; }
log_verbose() { log verbose "${BLUE}" "$*"; }

die() {
  log_error "$*"
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

####
# Copyright (c) 2016-2025
#   Jakob Westhoff <jakob@westhoffswelt.de>
#
# Redistribution and use in source and binary forms, with or without
# modification, are permitted provided that the following conditions are met:
#
#  - Redistributions of source code must retain the above copyright notice, this
#    list of conditions and the following disclaimer.
#  - Redistributions in binary form must reproduce the above copyright notice,
#    this list of conditions and the following disclaimer in the documentation
#    and/or other materials provided with the distribution.
#
# THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS" AND
# ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE IMPLIED
# WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
# DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE
# FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL
# DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR
# SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER
# CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY,
# OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
# OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
#
# Embedded from https://github.com/jakobwesthoff/prettytable.sh
####

_prettytable_char_top_left="┌"
_prettytable_char_horizontal="─"
_prettytable_char_vertical="│"
_prettytable_char_bottom_left="└"
_prettytable_char_bottom_right="┘"
_prettytable_char_top_right="┐"
_prettytable_char_vertical_horizontal_left="├"
_prettytable_char_vertical_horizontal_right="┤"
_prettytable_char_vertical_horizontal_top="┬"
_prettytable_char_vertical_horizontal_bottom="┴"
_prettytable_char_vertical_horizontal="┼"

_prettytable_color_blue="0;34"
_prettytable_color_green="0;32"
_prettytable_color_cyan="0;36"
_prettytable_color_red="0;31"
_prettytable_color_purple="0;35"
_prettytable_color_yellow="0;33"
_prettytable_color_gray="1;30"
_prettytable_color_light_blue="1;34"
_prettytable_color_light_green="1;32"
_prettytable_color_light_cyan="1;36"
_prettytable_color_light_red="1;31"
_prettytable_color_light_purple="1;35"
_prettytable_color_light_yellow="1;33"
_prettytable_color_light_gray="0;37"
_prettytable_color_black="0;30"
_prettytable_color_white="1;37"
_prettytable_color_none="0"

_prettytable_prettify_lines() {
  sed -e "s@^@${_prettytable_char_vertical}@;s@\$@	@;s@	@	${_prettytable_char_vertical}@g"
}

_prettytable_fill_border_lines() {
  sed -e "1s@ @${_prettytable_char_horizontal}@g;3s@ @${_prettytable_char_horizontal}@g;\$s@ @${_prettytable_char_horizontal}@g"
}

_prettytable_colorize_lines() {
  local color="$1"
  local range="$2"
  local color_var="_prettytable_color_${color}"
  local ansicolor="${!color_var}"

  sed -e "${range}s@\([^${_prettytable_char_vertical}]\{1,\}\)@"$'\E'"[${ansicolor}m\1"$'\E'"[${_prettytable_color_none}m@g"
}

prettytable() {
  local cols="$1"
  local color="${2:-none}"
  local input header body i
  input="$(cat)"
  header="$(head -n1 <<<"${input}")"
  body="$(tail -n+2 <<<"${input}")"
  {
    printf '%s' "${_prettytable_char_top_left}"
    for ((i = 2; i <= cols; i++)); do
      printf '\t%s' "${_prettytable_char_vertical_horizontal_top}"
    done
    printf '\t%s\n' "${_prettytable_char_top_right}"

    printf '%s\n' "${header}" | _prettytable_prettify_lines

    printf '%s' "${_prettytable_char_vertical_horizontal_left}"
    for ((i = 2; i <= cols; i++)); do
      printf '\t%s' "${_prettytable_char_vertical_horizontal}"
    done
    printf '\t%s\n' "${_prettytable_char_vertical_horizontal_right}"

    printf '%s\n' "${body}" | _prettytable_prettify_lines

    printf '%s' "${_prettytable_char_bottom_left}"
    for ((i = 2; i <= cols; i++)); do
      printf '\t%s' "${_prettytable_char_vertical_horizontal_bottom}"
    done
    printf '\t%s\n' "${_prettytable_char_bottom_right}"
  } | column -o '' -t -s $'\t' | _prettytable_fill_border_lines | _prettytable_colorize_lines "${color}" "2"
}

main() {
  need_cmd curl
  need_cmd tar
  need_cmd sha256sum
  need_cmd install
  need_cmd mktemp
  need_cmd column

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

  log_info "latest release: ${tag}"

  local current=""
  if current="$(installed_version)"; then
    if [[ "$(normalize_ver "${current}")" == "${ver}" ]]; then
      log_info "fcvm ${current} already installed, skipping binary download"
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

    log_verbose "archive url: ${archive_url}"
    log_info "downloading ${archive_name}"
    curl -fsSL -o "${tmp}/${archive_name}" "${archive_url}"
    curl -fsSL -o "${tmp}/${sums_name}" "${sums_url}"

    expected="$(checksum_for "${tmp}/${sums_name}" "${archive_name}")"
    verify_sha256 "${tmp}/${archive_name}" "${expected}"

    tar -xzf "${tmp}/${archive_name}" -C "${tmp}"
    [[ -f "${tmp}/fcvm" ]] || die "archive did not contain fcvm binary"
    run_install "${tmp}/fcvm" "${FCVM_BIN}"
    log_info "installed fcvm ${ver} to ${FCVM_BIN}"

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

  log_info "downloading firecracker and jailer"
  fcvm download firecracker

  if [[ -e "${kernel_path}" ]]; then
    log_info "kernel already present at ${kernel_path}, skipping"
  else
    log_info "downloading kernel"
    fcvm download kernel
  fi

  printf '\n'
  local table_color="none"
  if [[ -t 1 ]] && [[ ! -v NO_COLOR ]]; then
    table_color="green"
  fi
  {
    printf 'Dependency\tPath\tStatus\n'
    printf 'fcvm\t%s\t%s\n' "${FCVM_BIN}" "$(status_of "${FCVM_BIN}")"
    printf 'firecracker\t%s\t%s\n' "${firecracker_bin}" "$(status_of "${firecracker_bin}")"
    printf 'jailer\t%s\t%s\n' "${jailer_bin}" "$(status_of "${jailer_bin}")"
    printf 'kernel\t%s\t%s\n' "${kernel_path}" "$(status_of "${kernel_path}")"
    printf 'rootfs\t%s\t%s\n' "${rootfs_path}" "$(status_of "${rootfs_path}")"
    if [[ -n "${docker_bin}" ]]; then
      printf 'docker\t%s\tpresent\n' "${docker_bin}"
    else
      printf 'docker\t(not found)\tmissing\n'
    fi
    printf 'kvm\t/dev/kvm\t%s\n' "$(status_of /dev/kvm)"
    printf 'ip\t%s\t%s\n' "$(tool_path ip)" "$(tool_status ip)"
    printf 'iptables\t%s\t%s\n' "$(tool_path iptables)" "$(tool_status iptables)"
    printf 'mkfs.ext4\t%s\t%s\n' "$(tool_path mkfs.ext4)" "$(tool_status mkfs.ext4)"
    printf 'truncate\t%s\t%s\n' "$(tool_path truncate)" "$(tool_status truncate)"
  } | prettytable 3 "${table_color}"

  printf '\n'
  if [[ -z "${docker_bin}" ]]; then
    log_warn "Docker is not installed. Install Docker to build a custom rootfs."
  fi
  log_info "Build a custom rootfs with:"
  log_info "  sudo fcvm build-rootfs --dockerfile ./Dockerfile"
}

main "$@"
