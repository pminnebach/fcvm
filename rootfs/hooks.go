package rootfs

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

const initEnvScript = `#!/bin/sh
set -e
MMDS=169.254.169.254
TOKEN=$(curl -s -X PUT "http://${MMDS}/latest/api/token" -H "X-metadata-token-ttl-seconds: 60" 2>/dev/null || true)
HDR=""
[ -n "$TOKEN" ] && HDR="-H X-metadata-token: $TOKEN"
curl -s $HDR "http://${MMDS}/latest/meta-data/env" -o /tmp/fcvm-env.json 2>/dev/null || exit 0
[ -s /tmp/fcvm-env.json ] || exit 0
mkdir -p /etc/fcvm
# ponytail: naive KEY=VAL parse; upgrade path: jq
grep -o '"[^"]*"[[:space:]]*:[[:space:]]*"[^"]*"' /tmp/fcvm-env.json | while read -r pair; do
  key=$(echo "$pair" | cut -d'"' -f2)
  val=$(echo "$pair" | cut -d'"' -f4)
  echo "export ${key}=\"${val}\"" >> /etc/fcvm/env
done
`

const mountsScript = `#!/bin/sh
set -e
MMDS=169.254.169.254
TOKEN=$(curl -s -X PUT "http://${MMDS}/latest/api/token" -H "X-metadata-token-ttl-seconds: 60" 2>/dev/null || true)
HDR=""
[ -n "$TOKEN" ] && HDR="-H X-metadata-token: $TOKEN"
curl -s $HDR "http://${MMDS}/latest/meta-data/mounts" -o /tmp/fcvm-mounts.json 2>/dev/null || exit 0
[ -s /tmp/fcvm-mounts.json ] || exit 0
# mounts applied by guest agent reading MMDS at boot; see fcvm-start.sh
`

const startScript = `#!/bin/sh
set -e
/usr/local/bin/fcvm-init-env || true
# network: default route via tap gateway
GW="${FCVM_GATEWAY:-172.16.0.1}"
ip link set eth0 up 2>/dev/null || true
ip addr add "${FCVM_GUEST_IP:-172.16.0.2}/30" dev eth0 2>/dev/null || true
ip route add default via "$GW" dev eth0 2>/dev/null || true
echo nameserver 8.8.8.8 > /etc/resolv.conf
# NFS mounts from MMDS
MMDS=169.254.169.254
TOKEN=$(curl -s -X PUT "http://${MMDS}/latest/api/token" -H "X-metadata-token-ttl-seconds: 60" 2>/dev/null || true)
HDR=""
[ -n "$TOKEN" ] && HDR="-H X-metadata-token: $TOKEN"
curl -s $HDR "http://${MMDS}/latest/meta-data/mounts" -o /tmp/fcvm-mounts.json 2>/dev/null || true
if [ -s /tmp/fcvm-mounts.json ]; then
  grep -o '"guest"[[:space:]]*:[[:space:]]*"[^"]*"' /tmp/fcvm-mounts.json | cut -d'"' -f4 | while read -r gp; do
    grep -B5 "\"guest\"[[:space:]]*:[[:space:]]*\"$gp\"" /tmp/fcvm-mounts.json | grep '"host"' | head -1 | cut -d'"' -f4 | while read -r hp; do
      mkdir -p "$gp"
      mount -t nfs -o nolock "$hp" "$gp" 2>/dev/null || true
    done
  done
fi
# block device mounts (secondary virtio drive)
for dev in /dev/vdb /dev/vdc; do
  [ -b "$dev" ] && mkdir -p /mnt/block && mount "$dev" /mnt/block 2>/dev/null && break
done
`

const profileScript = `[ -f /etc/fcvm/env ] && . /etc/fcvm/env
`

func InjectHooks(rootDir string) error {
	files := map[string]string{
		"usr/local/bin/fcvm-init-env":  initEnvScript,
		"usr/local/bin/fcvm-mounts.sh": mountsScript,
		"usr/local/bin/fcvm-start.sh":  startScript,
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
	// systemd oneshot or rc.local
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
	dir := filepath.Join(rootDir, "root/.ssh")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	authKeys := filepath.Join(dir, "authorized_keys")
	return os.WriteFile(authKeys, []byte(pubKey+"\n"), 0o600)
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
