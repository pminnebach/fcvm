package rootfs

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

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

const initEnvScript = `#!/bin/sh
. /usr/local/bin/fcvm-mmds.sh
fcvm_mmds_get latest/meta-data/env /tmp/fcvm-env.json 2>/dev/null || exit 0
[ -s /tmp/fcvm-env.json ] || exit 0
mkdir -p /etc/fcvm
: > /etc/fcvm/env
# ponytail: naive KEY=VAL parse; upgrade path: jq
grep -o '"[^"]*"[[:space:]]*:[[:space:]]*"[^"]*"' /tmp/fcvm-env.json | while read -r pair; do
  key=$(echo "$pair" | cut -d'"' -f2)
  val=$(echo "$pair" | cut -d'"' -f4)
  echo "export ${key}=\"${val}\"" >> /etc/fcvm/env
done
`

const mountsScript = `#!/bin/sh
. /usr/local/bin/fcvm-mmds.sh
fcvm_mmds_get latest/meta-data/mounts /tmp/fcvm-mounts.json 2>/dev/null || exit 0
[ -s /tmp/fcvm-mounts.json ] || exit 0
# mounts applied by guest agent reading MMDS at boot; see fcvm-start.sh
`

const applyMountsScript = `#!/bin/sh
. /usr/local/bin/fcvm-mmds.sh
fcvm_mmds_route
fcvm_mmds_get latest/meta-data/mounts /tmp/fcvm-mounts.json 2>/dev/null || true
[ -s /tmp/fcvm-mounts.json ] || exit 0
grep -o '"guest"[[:space:]]*:[[:space:]]*"[^"]*"' /tmp/fcvm-mounts.json | cut -d'"' -f4 | while read -r gp; do
  chunk=$(grep -B5 "\"guest\"[[:space:]]*:[[:space:]]*\"$gp\"" /tmp/fcvm-mounts.json | tail -6)
  method=$(echo "$chunk" | sed -n 's/.*"method"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -1)
  hp=$(echo "$chunk" | sed -n 's/.*"host"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -1)
  [ "$method" = "block" ] && continue
  mkdir -p "$gp"
  mount -t nfs -o nolock "$hp" "$gp" 2>/dev/null || true
done
block_i=0
for gp in $(grep -B3 '"method"[[:space:]]*:[[:space:]]*"block"' /tmp/fcvm-mounts.json | sed -n 's/.*"guest"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p'); do
  dev=""
  [ "$block_i" -eq 0 ] && [ -b /dev/vdb ] && dev=/dev/vdb
  [ "$block_i" -eq 1 ] && [ -b /dev/vdc ] && dev=/dev/vdc
  [ "$block_i" -eq 2 ] && [ -b /dev/vdd ] && dev=/dev/vdd
  block_i=$((block_i + 1))
  [ -n "$dev" ] || continue
  mkdir -p "$gp"
  mount "$dev" "$gp" 2>/dev/null || true
done
`

const startScript = `#!/bin/sh
[ -f /etc/fcvm/network ] && . /etc/fcvm/network
. /usr/local/bin/fcvm-mmds.sh
IF="${FCVM_IFACE:-eth0}"
if ! ip link show "$IF" >/dev/null 2>&1; then
  IF=$(ip -o link show | awk -F': ' '$2 != "lo" {print $2; exit}')
fi
[ -n "$IF" ] || IF=eth0
mkdir -p /root/.ssh
chmod 700 /root /root/.ssh
[ -f /root/.ssh/authorized_keys ] && chmod 600 /root/.ssh/authorized_keys
if ! ip -o -4 addr show dev "$IF" scope global 2>/dev/null | grep -q .; then
  GW="${FCVM_GATEWAY:-172.16.0.1}"
  ip link set "$IF" up 2>/dev/null || true
  ip addr add "${FCVM_GUEST_IP:-172.16.0.2}/30" dev "$IF" 2>/dev/null || true
  ip route add default via "$GW" dev "$IF" 2>/dev/null || true
fi
fcvm_mmds_route
echo nameserver 8.8.8.8 > /etc/resolv.conf
/usr/local/bin/fcvm-apply-mounts.sh || true
/usr/local/bin/fcvm-init-env || true
`

const profileScript = `if [ ! -s /etc/fcvm/env ]; then
  /usr/local/bin/fcvm-init-env 2>/dev/null || true
fi
[ -f /etc/fcvm/env ] && . /etc/fcvm/env
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
		"usr/local/bin/fcvm-mmds.sh":        mmdsHelpers,
		"usr/local/bin/fcvm-init-env":     initEnvScript,
		"usr/local/bin/fcvm-mounts.sh":    mountsScript,
		"usr/local/bin/fcvm-apply-mounts.sh": applyMountsScript,
		"usr/local/bin/fcvm-start.sh":     startScript,
		"etc/profile.d/fcvm.sh":        profileScript,
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
	if err := os.WriteFile(rc, []byte(rcContent), 0o755); err != nil {
		return err
	}
	return nil
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

func InjectNetwork(rootDir, guestIP, tapIP string) error {
	dir := filepath.Join(rootDir, "etc/fcvm")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	content := fmt.Sprintf("FCVM_GUEST_IP=%s\nFCVM_GATEWAY=%s\nFCVM_IFACE=eth0\n", guestIP, tapIP)
	return os.WriteFile(filepath.Join(dir, "network"), []byte(content), 0o644)
}

func PatchMounted(mountPoint, ext4Path, sshPubKey string) error {
	if out, err := exec.Command("mount", "-o", "loop", ext4Path, mountPoint).CombinedOutput(); err != nil {
		return fmt.Errorf("mount ext4: %s: %w", out, err)
	}
	defer exec.Command("umount", mountPoint).Run()
	if err := InjectHooks(mountPoint); err != nil {
		return err
	}
	if sshPubKey != "" {
		if err := InjectSSHKey(mountPoint, sshPubKey); err != nil {
			return err
		}
	}
	return nil
}
