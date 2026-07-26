# Debugging from the host

How to find and inspect fcvm's microVMs using only standard Linux tools — no `fcvm` binary involved. Use this when the CLI will not run, when `state.json` is gone or wrong, or when you want to confirm what fcvm actually did to the host.

Everything here is read-only unless the section says otherwise. Run as root; the jailer chroots and the state directory are root-owned.

Throughout, `$STATE_DIR` is the state directory — `~/.fcvm` by default, and under `sudo` that is the **invoking user's** home (fcvm honours `SUDO_USER`), so it is usually `/home/you/.fcvm`, not `/root/.fcvm`.

```bash
STATE_DIR=~/.fcvm      # or /home/you/.fcvm when you sudo'd
JAIL_ROOT=$STATE_DIR/jailer/firecracker
```

## Quick inventory

Two independent ways to enumerate VMs. They can disagree, and the disagreement is itself the diagnosis (see [orphans](#finding-orphans)).

**From running processes** — authoritative for what is actually running, needs no state files:

```bash
for pid in $(pgrep -x firecracker); do
  id=$(tr '\0' '\n' < /proc/$pid/cmdline | grep -A1 -x -- --id | tail -1)
  jail=$(awk '$5=="/"{print $4; exit}' /proc/$pid/mountinfo)
  rss=$(awk '/^VmRSS/{print $2}' /proc/$pid/status)
  printf 'pid=%-8s id=%-14s rss=%-9s jail=%s\n' "$pid" "$id" "${rss}kB" "$jail"
done
```

```
pid=33736    id=manual1        rss=178428kB  jail=/root/.fcvm/jailer/firecracker/manual1/root
```

**From the jail directories** — shows VMs fcvm believes exist, including dead ones:

```bash
for d in "$JAIL_ROOT"/*/; do
  id=$(basename "$d")
  pid=$(cat "$d/root/firecracker.pid" 2>/dev/null || echo -)
  alive=no; [ "$pid" != - ] && kill -0 "$pid" 2>/dev/null && alive=yes
  printf '%-14s pid=%-8s alive=%s\n' "$id" "$pid" "$alive"
done
```

## Processes and jails

The process you see is **firecracker, not jailer**: the jailer sets up the chroot, drops privileges and then `execve`s firecracker, so it replaces itself.

```bash
pgrep -a firecracker
```

```
33736 /firecracker --id manual1 --start-time-us 4652668169 --start-time-cpu-us 7544 --api-sock firecracker.sock
```

Two things to note in that line:

- **`--id <vmid>` is the VM id.** This is the most reliable process→VM mapping.
- **`argv[0]` is `/firecracker`** — a path inside the chroot, not on your filesystem.

| Want | Command |
|------|---------|
| VM id of a pid | `tr '\0' '\n' < /proc/$PID/cmdline \| grep -A1 -x -- --id \| tail -1` |
| Jail directory of a pid | `awk '$5=="/"{print $4; exit}' /proc/$PID/mountinfo` |
| Memory actually in use | `grep VmRSS /proc/$PID/status` |
| Jailer uid/gid it dropped to | `grep -E '^[UG]id' /proc/$PID/status` |
| Open files (jail-relative paths) | `ls -l /proc/$PID/fd \| grep -v anon_inode` |
| cgroup | `cat /proc/$PID/cgroup` |

**`/proc/$PID/root` is not useful here.** It reads as `/` because the jailer gives the VM its own mount namespace and pivots into the chroot. Use the `mountinfo` command above instead — field 4 of the root mount is the jail path as seen from the host:

```
226 119 254:0 /root/.fcvm/jailer/firecracker/manual1/root / rw,relatime master:1 - ext4 /dev/root rw
                └── the jail directory                    └── it is "/" to the VM
```

Open file descriptors are likewise jail-relative, which is how you confirm which disk image a VM has open:

```
13 -> /rootfs.ext4
17 -> /dev/kvm
```

## Inside a VM's jail

```bash
ls -l "$JAIL_ROOT"/<id>/root/
```

```
drwx------ 3 root root      4096 dev            # kvm, net/tun, urandom, userfaultfd
-rwxr-xr-x 1 root root   3527456 firecracker    # copy of the binary
-rw-r--r-- 1 root root         5 firecracker.pid
srwxr-xr-x 1 root root         0 firecracker.sock
-rw-r--r-- 1 root root         0 firecracker.log
-rw-r--r-- 2 root root 543302620 rootfs.ext4    # link count 2 — see below
-rw-r--r-- 2 root root  27707728 vmlinux
```

`firecracker.pid` gives you the pid without `ps`, and the directory name gives you the id — that pair is the whole mapping.

**Disk images in the jail are hard links, not copies.** The SDK links `$STATE_DIR/vms/<id>/rootfs.ext4` and each `mount-<n>.ext4` into the jail (hence link count 2). Both paths are the same inode, so reading either one sees the guest's writes, and deleting the jail does not destroy the image.

```bash
stat -c '%i %h %n' "$STATE_DIR"/vms/<id>/rootfs.ext4 "$JAIL_ROOT"/<id>/root/rootfs.ext4
```

Matching inode numbers confirm the link.

## Talking to a live VM

Every VM has a Firecracker API socket in its jail. You can query a running VM directly, which is the ground truth for its configuration:

```bash
SOCK=$JAIL_ROOT/<id>/root/firecracker.sock
curl -s --unix-socket "$SOCK" http://localhost/vm/config
curl -s --unix-socket "$SOCK" http://localhost/machine-config
```

```json
{"vcpu_count":1,"mem_size_mib":256,"smt":false,"track_dirty_pages":false,"huge_pages":"None"}
```

Confirm the socket is live and who owns it:

```bash
ss -xlp | grep firecracker.sock
```

```
u_str LISTEN 0 4096 firecracker.sock 46138 * 0 users:(("firecracker",pid=33736,fd=6))
```

Note `/vm/config` reports paths **as the VM sees them** (`rootfs.ext4`, `vmlinux`), because they are inside the chroot.

`PUT` requests to this socket will change or stop a running VM. Stick to `GET` unless that is what you want.

## State files

fcvm's own view lives in `$STATE_DIR/vms/<id>/state.json`. Read it directly:

```bash
cat "$STATE_DIR"/vms/*/state.json
```

A table across all VMs, without `fcvm` and without `jq`:

```bash
python3 - <<'PY'
import json, glob, os
for f in sorted(glob.glob(os.path.expanduser('~/.fcvm/vms/*/state.json'))):
    s = json.load(open(f))
    alive = os.path.exists('/proc/%d' % s['pid'])
    print(f"{s['id']:<12} idx={s.get('index')} pid={s['pid']:<7} alive={alive!s:<5} "
          f"ip={s.get('guest_ip','-'):<14} tap={s.get('tap_dev') or '-':<12} {s.get('network_mode','tap')}")
PY
```

```
demo         idx=0 pid=33736   alive=True  ip=172.16.0.2   tap=fcvm-tap-0   tap
```

Fields worth knowing when debugging:

| Field | Why it matters |
|-------|----------------|
| `index` | Drives the TAP name, the `/30`, and the per-VM jailer uid. Two VMs sharing one is a bug |
| `pid` + `pid_start` | `pid_start` is field 22 of `/proc/<pid>/stat`; fcvm compares it before signalling so it never kills a recycled pid |
| `tap_dev`, `host_iface`, `guest_subnet` | Exactly what teardown will remove from the firewall |
| `mounts[].method` / `.device` | `block` mounts are copies synced back on stop; `.device` is the image holding the guest's writes |

To check the pid identity by hand — this is the comparison `fcvm list` makes:

```bash
sed 's/.*) //' /proc/$PID/stat | awk '{print $20}'   # 465266
```

If that number differs from `pid_start` in `state.json`, the pid was recycled and the VM is **gone**, whatever the state file says.

## Networking

### TAP devices and addresses

```bash
ip -d link show type tun          # every TAP on the host
ip -br link show | grep fcvm-tap  # just fcvm's
ip -br addr show | grep fcvm-tap  # with their /30 host addresses
```

fcvm names TAPs `fcvm-tap-<index>`, and the `/30` is derived by shifting the **third octet** of `network.tap-ip` by that index: index 0 → `172.16.0.1/30` host, `172.16.0.2` guest; index 1 → `172.16.1.1` / `172.16.1.2`.

```bash
ip route            # per-/30 routes
cat /proc/sys/net/ipv4/ip_forward
cat /proc/sys/net/ipv4/conf/fcvm-tap-0/proxy_arp
ip -j route list default    # the egress interface fcvm NATs to
```

### Firewall rules

fcvm never touches the `FORWARD` policy. All of its rules live in a dedicated `FCVM` chain plus one NAT rule per VM:

```bash
iptables -S FCVM                          # per-VM accept rules
iptables -S FORWARD                       # should contain "-A FORWARD -j FCVM"
iptables -t nat -S POSTROUTING | grep MASQUERADE
iptables -L FCVM -n -v --line-numbers     # with packet counters
```

```
-N FCVM
-A FCVM -i fcvm-tap-0 -o eth0 -j ACCEPT
-A FCVM -i eth0 -o fcvm-tap-0 -m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT
-A POSTROUTING -s 172.16.0.0/30 -o eth0 -j MASQUERADE
```

Three rules per running VM. The packet counters in the `-L -v` output are the fastest way to tell whether guest traffic is actually flowing.

On an nftables host, `iptables` is usually the `iptables-nft` shim and the above still works; `nft list ruleset` shows the same rules in native form.

### CNI mode

When `network.cni-network` is set there is no TAP and no `FCVM` chain — the plugins own the rules.

```bash
ip netns list                       # one netns per VM, named after the VM id
ls -l /var/run/netns/
ls /var/lib/cni/                    # per-VM CNI cache
ip netns exec <id> ip -br addr      # addresses inside the VM's netns
ip netns exec <id> ip route
```

## Host mounts

### NFS exports

fcvm bind-mounts each shared host directory under the state dir and exports it to the guest address only:

```bash
ls -l "$STATE_DIR"/exports/*/share       # one per <id>-<mount index>
cat /etc/exports.d/fcvm-*.exports
exportfs -v                              # what the kernel actually serves
showmount -e localhost
```

```
/home/you/.fcvm/exports/vm-0/share 172.16.0.2(rw,sync,no_subtree_check,all_squash,anonuid=1000,anongid=1000)
```

The client field should be a **single guest IP**. If you see `*`, that export is reachable by every host that can talk to port 2049 — it predates the export-scoping fix.

Find the bind mounts themselves:

```bash
findmnt -o TARGET,SOURCE,FSTYPE,OPTIONS | grep fcvm
grep 'fcvm/exports' /proc/self/mountinfo
```

```
120 23 0:31 /hostdata /home/you/.fcvm/exports/demo-0/share rw,nosuid,nodev shared:17 - tmpfs tmpfs rw
            └── real host directory   └── where it is bound
```

Field 4 is the host directory the guest is really writing to. That is the line to check before deleting anything under `exports/` — see the warning below.

### Block-device mounts

`method=block` mounts are ext4 images, not live mounts:

```bash
ls -l "$STATE_DIR"/vms/<id>/mount-*.ext4
```

They map to `/dev/vdb`, `/dev/vdc`, … in the guest in slot order (`/dev/vda` is the rootfs). To read what the guest wrote without booting it:

```bash
mkdir -p /mnt/inspect
mount -o loop,ro "$STATE_DIR"/vms/<id>/mount-0.ext4 /mnt/inspect
ls /mnt/inspect
umount /mnt/inspect
```

Mount read-only while the VM is running. These images are only mirrored back to the host directory on `stop`/`cleanup`, so a VM that was killed still has the guest's writes sitting here.

## Guest access without the CLI

```bash
ssh -i "$STATE_DIR"/id_ed25519 \
    -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null \
    root@<guest_ip>
```

The guest IP is in `state.json`, or `ip -br addr show fcvm-tap-N` plus one (the host holds `.1`, the guest `.2`). Inside the guest, the host-written config files are:

| Path | Contents |
|------|----------|
| `/etc/fcvm/network` | `FCVM_GUEST_IP`, `FCVM_GATEWAY`, `FCVM_IFACE`, `FCVM_NAMESERVERS` |
| `/etc/fcvm/env` | injected env, shell-quoted |
| `/etc/fcvm/mounts` | mount table: `method`, `source`, `guest path`, tab-separated |

```bash
systemctl status fcvm-start.service   # in the guest
journalctl -u fcvm-start.service
```

## Logs

```bash
tail -f "$JAIL_ROOT"/<id>/root/firecracker.log
```

That file is the guest serial console plus Firecracker's own log — kernel panics and boot failures land here. If the VM died before it was created, there is no log; check whatever ran `fcvm start` instead.

## Finding orphans

Orphans are the cases where the two inventories disagree.

**State without a process** — VM crashed or the host rebooted:

```bash
for f in "$STATE_DIR"/vms/*/state.json; do
  pid=$(python3 -c "import json,sys;print(json.load(open('$f'))['pid'])")
  kill -0 "$pid" 2>/dev/null || echo "dead: $f (pid $pid)"
done
```

**TAP without a VM** — teardown was interrupted:

```bash
comm -13 \
  <(pgrep -x firecracker | xargs -r -I{} sh -c "tr '\0' '\n' < /proc/{}/cmdline | grep -A1 -x -- --id | tail -1" | sort) \
  <(ls "$JAIL_ROOT" 2>/dev/null | sort)
```

Anything listed has a jail directory but no running process.

**Rules or exports without a VM:**

```bash
iptables -S FCVM | grep -o 'fcvm-tap-[0-9]*' | sort -u    # vs. ip link show
ls /etc/exports.d/fcvm-*.exports                          # vs. $STATE_DIR/vms/
```

## Manual cleanup

Prefer `fcvm cleanup <id>` / `fcvm cleanup --all`. Do this by hand only when the CLI cannot run, and in this order.

> **Unmount the export share before deleting anything under `exports/`.** `rm -rf` over a live bind mount deletes the contents of the *host* directory it points at. Check `findmnt` first, every time. fcvm itself refuses to remove a directory that is still a mount point for exactly this reason.

```bash
ID=<vmid>

# 1. Stop the VM.
PID=$(cat "$JAIL_ROOT/$ID/root/firecracker.pid")
kill -TERM "$PID"; sleep 2; kill -0 "$PID" 2>/dev/null && kill -KILL "$PID"

# 2. Unexport and unmount, in that order.
rm -f /etc/exports.d/fcvm-$ID-*.exports
exportfs -ra
for s in "$STATE_DIR"/exports/$ID-*/share; do
  mountpoint -q "$s" && umount "$s"
done
findmnt | grep fcvm || echo "no fcvm mounts left"      # verify before removing
rm -rf "$STATE_DIR"/exports/$ID-*

# 3. Firewall and TAP. Read tap_dev/guest_subnet/host_iface from state.json.
TAP=fcvm-tap-0; SUBNET=172.16.0.0/30; EGRESS=eth0
iptables -D FCVM -i $TAP -o $EGRESS -j ACCEPT
iptables -D FCVM -i $EGRESS -o $TAP -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT
iptables -t nat -D POSTROUTING -s $SUBNET -o $EGRESS -j MASQUERADE
ip link del $TAP

# 4. Jail and state. Recover block-mount data first if you need it.
rm -rf "$JAIL_ROOT/$ID" "$STATE_DIR/vms/$ID"
```

Once the last VM is gone, remove the shared bits too:

```bash
iptables -D FORWARD -j FCVM; iptables -F FCVM; iptables -X FCVM
cat "$STATE_DIR"/.ip_forward.orig > /proc/sys/net/ipv4/ip_forward   # fcvm's saved value
rm -f "$STATE_DIR"/.ip_forward.orig "$STATE_DIR"/.lock
```

In CNI mode, replace step 3 with `ip netns del $ID` and `rm -rf /var/lib/cni/$ID`.

## Related docs

- [architecture.md](architecture.md) — state layout and lifecycle
- [network.md](network.md) — what the TAP, chain and export setup is meant to look like
- [cli.md](cli.md) — the supported commands for all of the above
