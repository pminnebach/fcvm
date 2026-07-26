package rootfs

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// resolvMarker identifies an /etc/resolv.conf that fcvm wrote, so the guest
// bootstrap can refresh its own file without clobbering an operator's.
const resolvMarker = "# managed by fcvm"

const mmdsHelpers = `#!/bin/sh
MMDS=169.254.169.254

fcvm_mmds_route() {
  IF="${FCVM_IFACE:-eth0}"
  [ -f /etc/fcvm/network ] && . /etc/fcvm/network
  ip route add "$MMDS" dev "$IF" 2>/dev/null || true
}

fcvm_mmds_get() {
  path=$1
  out=$2
  fcvm_mmds_route
  token=$(curl -sf -X PUT "http://${MMDS}/latest/api/token" \
    -H "X-metadata-token-ttl-seconds: 60") || return 1
  curl -sf -H "X-metadata-token: $token" -H "Accept: application/json" \
    "http://${MMDS}/${path}" -o "$out"
}
`

// applyMountsScript reads the mount table the host wrote, one record per line
// with tab-separated method, source and guest path. The host already knows all
// of this, so the guest does no parsing beyond splitting fields.
const applyMountsScript = `#!/bin/sh
[ -f /etc/fcvm/mounts ] || exit 0
tab=$(printf '\t')
while IFS="$tab" read -r method source guest; do
  [ -n "$guest" ] || continue
  mkdir -p "$guest" || continue
  case "$method" in
    nfs)
      mount -t nfs -o nolock "$source" "$guest" || echo "fcvm: nfs mount $source -> $guest failed" >&2
      ;;
    block)
      if [ -b "$source" ]; then
        mount "$source" "$guest" || echo "fcvm: mount $source -> $guest failed" >&2
      else
        echo "fcvm: block device $source missing for $guest" >&2
      fi
      ;;
  esac
done < /etc/fcvm/mounts
`

const startScript = `#!/bin/sh
[ -f /etc/fcvm/network ] && . /etc/fcvm/network
IF="${FCVM_IFACE:-eth0}"
if ! ip link show "$IF" >/dev/null 2>&1; then
  IF=$(ip -o link show | awk -F': ' '$2 != "lo" {print $2; exit}')
fi
[ -n "$IF" ] || IF=eth0
mkdir -p /root/.ssh
chmod 700 /root /root/.ssh
[ -f /root/.ssh/authorized_keys ] && chmod 600 /root/.ssh/authorized_keys
if ! ip -o -4 addr show dev "$IF" scope global 2>/dev/null | grep -q .; then
  ip link set "$IF" up 2>/dev/null || true
  if [ -n "$FCVM_GUEST_IP" ]; then
    ip addr add "${FCVM_GUEST_IP}/30" dev "$IF" 2>/dev/null || true
  fi
  if [ -n "$FCVM_GATEWAY" ]; then
    ip route add default via "$FCVM_GATEWAY" dev "$IF" 2>/dev/null || true
  fi
fi
[ -f /usr/local/bin/fcvm-mmds.sh ] && . /usr/local/bin/fcvm-mmds.sh && fcvm_mmds_route
# Only write resolv.conf if it is missing or was written by us.
if [ -n "$FCVM_NAMESERVERS" ]; then
  if [ ! -s /etc/resolv.conf ] || head -n 1 /etc/resolv.conf | grep -q 'managed by fcvm'; then
    { echo '` + resolvMarker + `'
      for ns in $FCVM_NAMESERVERS; do echo "nameserver $ns"; done
    } > /etc/resolv.conf
  fi
fi
/usr/local/bin/fcvm-apply-mounts.sh || true
`

const profileScript = `[ -f /etc/fcvm/env ] && . /etc/fcvm/env
`

const systemdUnit = `[Unit]
Description=fcvm guest bootstrap
After=network.target
DefaultDependencies=yes

[Service]
Type=oneshot
ExecStart=/usr/local/bin/fcvm-start.sh
RemainAfterExit=yes

[Install]
WantedBy=multi-user.target
`

func InjectHooks(rootDir string) error {
	files := map[string]string{
		"usr/local/bin/fcvm-mmds.sh":         mmdsHelpers,
		"usr/local/bin/fcvm-apply-mounts.sh": applyMountsScript,
		"usr/local/bin/fcvm-start.sh":        startScript,
		"etc/profile.d/fcvm.sh":              profileScript,
	}
	for rel, content := range files {
		path := filepath.Join(rootDir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
			return err
		}
	}
	// systemd oneshot (debuerreotype ignores rc.local) + rc.local fallback
	unitDir := filepath.Join(rootDir, "etc/systemd/system")
	if err := os.MkdirAll(unitDir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(unitDir, "fcvm-start.service"), []byte(systemdUnit), 0o644); err != nil {
		return err
	}
	wantDir := filepath.Join(rootDir, "etc/systemd/system/multi-user.target.wants")
	if err := os.MkdirAll(wantDir, 0o755); err != nil {
		return err
	}
	link := filepath.Join(wantDir, "fcvm-start.service")
	_ = os.Remove(link)
	if err := os.Symlink("../fcvm-start.service", link); err != nil {
		return err
	}
	rc := filepath.Join(rootDir, "etc/rc.local")
	rcContent := `#!/bin/sh
/usr/local/bin/fcvm-start.sh
exit 0
`
	return os.WriteFile(rc, []byte(rcContent), 0o755)
}

func InjectSSHKey(rootDir, pubKey string) error {
	rootHome := filepath.Join(rootDir, "root")
	if err := os.MkdirAll(rootHome, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(rootHome, 0o700); err != nil {
		return err
	}
	dir := filepath.Join(rootHome, ".ssh")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return err
	}
	authKeys := filepath.Join(dir, "authorized_keys")
	if err := os.WriteFile(authKeys, []byte(strings.TrimSpace(pubKey)+"\n"), 0o600); err != nil {
		return err
	}
	if err := os.Chmod(authKeys, 0o600); err != nil {
		return err
	}
	for _, p := range []string{rootHome, dir, authKeys} {
		if err := os.Chown(p, 0, 0); err != nil {
			return err
		}
	}
	return injectSSHConfig(rootDir)
}

func injectSSHConfig(rootDir string) error {
	dir := filepath.Join(rootDir, "etc/ssh/sshd_config.d")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	cfg := "PermitRootLogin prohibit-password\nPubkeyAuthentication yes\nPasswordAuthentication no\nAuthorizedKeysFile .ssh/authorized_keys\n"
	return os.WriteFile(filepath.Join(dir, "fcvm.conf"), []byte(cfg), 0o644)
}

// InjectNetwork writes the static guest network settings sourced by the boot
// hooks. guestIP and gateway may be empty in CNI mode, where the SDK
// configures the interface.
func InjectNetwork(rootDir, guestIP, gateway string, nameservers []string) error {
	dir := filepath.Join(rootDir, "etc/fcvm")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	content := fmt.Sprintf("FCVM_GUEST_IP=%s\nFCVM_GATEWAY=%s\nFCVM_IFACE=eth0\nFCVM_NAMESERVERS=%q\n",
		guestIP, gateway, strings.Join(nameservers, " "))
	return os.WriteFile(filepath.Join(dir, "network"), []byte(content), 0o644)
}

// InjectEnv writes /etc/fcvm/env, which /etc/profile.d sources. Values are
// single-quoted here rather than in shell so that quotes, backslashes and
// command substitutions in a value survive intact.
func InjectEnv(rootDir string, env map[string]string) error {
	dir := filepath.Join(rootDir, "etc/fcvm")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "env"), []byte(RenderEnv(env)), 0o644)
}

// RenderEnv formats env as sourceable shell assignments, sorted for stable output.
func RenderEnv(env map[string]string) string {
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&b, "export %s=%s\n", k, ShellQuote(env[k]))
	}
	return b.String()
}

// ShellQuote wraps s in single quotes so the shell treats it as a literal.
func ShellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// MountRecord is one line of the guest mount table.
type MountRecord struct {
	Method string // nfs or block
	Source string // "gateway:/export/path" or "/dev/vdb"
	Guest  string
}

// RenderMounts formats the mount table read by fcvm-apply-mounts.sh.
func RenderMounts(records []MountRecord) string {
	var b strings.Builder
	for _, r := range records {
		fmt.Fprintf(&b, "%s\t%s\t%s\n", r.Method, r.Source, r.Guest)
	}
	return b.String()
}

// PatchOptions is everything the host bakes into a per-VM rootfs before boot.
type PatchOptions struct {
	SSHPubKey   string
	Env         map[string]string
	Nameservers []string
	// StaticNetwork writes /etc/fcvm/network. Skipped in CNI mode, where the
	// SDK configures the interface and the addresses are not known yet.
	StaticNetwork bool
	GuestIP       string
	Gateway       string
}

// PatchMounted mounts an ext4 image, applies everything in opts, and unmounts
// it. The unmount error is returned rather than dropped: callers commonly
// remove the mount point afterwards, and doing that over a live mount deletes
// the mount source instead.
func PatchMounted(mountPoint, ext4Path string, opts PatchOptions) (err error) {
	if out, mErr := exec.Command("mount", "-o", "loop", ext4Path, mountPoint).CombinedOutput(); mErr != nil {
		return fmt.Errorf("mount ext4: %s: %w", out, mErr)
	}
	defer func() {
		if out, uErr := exec.Command("umount", mountPoint).CombinedOutput(); uErr != nil && err == nil {
			err = fmt.Errorf("umount %s: %s: %w", mountPoint, strings.TrimSpace(string(out)), uErr)
		}
	}()
	if err := InjectHooks(mountPoint); err != nil {
		return err
	}
	if opts.SSHPubKey != "" {
		if err := InjectSSHKey(mountPoint, opts.SSHPubKey); err != nil {
			return err
		}
	}
	if len(opts.Env) > 0 {
		if err := InjectEnv(mountPoint, opts.Env); err != nil {
			return err
		}
	}
	if opts.StaticNetwork {
		if err := InjectNetwork(mountPoint, opts.GuestIP, opts.Gateway, opts.Nameservers); err != nil {
			return err
		}
	} else if len(opts.Nameservers) > 0 {
		// CNI: no addresses to write, but DNS still comes from the host config.
		if err := InjectNetwork(mountPoint, "", "", opts.Nameservers); err != nil {
			return err
		}
	}
	return nil
}
