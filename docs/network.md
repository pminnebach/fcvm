# Networking

fcvm supports two guest networking modes. Default is static TAP + MASQUERADE. Set `network.cni-network` (or `--cni-network`) to use CNI.

Guest access after boot is SSH to the guest IP, or vsock via `fcvm vsock-exec`. Every VM gets a virtio-vsock device (guest CID 3, UDS `vsock.sock` in the jailer chroot).

## Vsock

Firecracker maps guest `AF_VSOCK` to host `AF_UNIX` under the jail root (`<chroot>/vsock.sock`).

| Direction | Mechanism |
|-----------|-----------|
| Host → guest (commands) | Connect to `vsock.sock`, send `CONNECT 5252`, write a command line |
| Guest → host (output) | Guest dials host CID 2 port 5253; Firecracker connects to `vsock.sock_5253` |

The guest runs `fcvm-guest-agent` (injected at rootfs patch time, systemd). Host kernel needs `CONFIG_VHOST_VSOCK`; guest needs `/dev/vsock` (`CONFIG_VIRTIO_VSOCKETS`, present on Firecracker CI kernels).

```bash
sudo ./fcvm vsock-exec myvm -- uname -a
```

Build and install the agent next to the configured path (default `~/.fcvm/bin/fcvm-guest-agent`):

```bash
go build -buildvcs=false -o ~/.fcvm/bin/fcvm-guest-agent ./guest/agent
```

## Static TAP (default)

When `cni-network` is empty:

1. Allocate a VM index and derive a `/30` from `network.tap-ip` / `network.guest-ip` by shifting the **third octet** by the index (`network.SubnetForIndex`).
2. Create TAP `fcvm-tap-N`, assign the host `/30`, enable `proxy_arp` and `ip_forward`, and add this VM's rules (see below).
3. Derive guest MAC as `06:00:` + guest IPv4 bytes (`GuestMAC`).
4. Patch the per-VM rootfs with `/etc/fcvm/network` (`FCVM_GUEST_IP`, `FCVM_GATEWAY`, `FCVM_IFACE=eth0`, `FCVM_NAMESERVERS`).
5. Firecracker NIC uses static config: host TAP, guest `/30`, gateway = tap IP, `network.nameservers` for DNS, MMDS allowed.
6. Stop/cleanup deletes this VM's rules and the TAP (`ip link del`).

Defaults: `tap-ip: 172.16.0.1`, `guest-ip: 172.16.0.2` (VM index 0). Index 1 → `172.16.1.1` / `172.16.1.2`, and so on. An index that would push the third octet past 255 is an error rather than a silent wrap onto another VM's address.

### Index allocation

The index is the **lowest one not claimed by an existing VM**, not a count of running VMs. Counting would reuse the index of a stopped VM and collide with VMs still running — and since the index also selects the TAP name and the per-VM jailer uid, that collision would take over a live VM's network. Allocation and the state write that claims the index are serialised with a lock file at `<state-dir>/.lock`.

The index is recorded as `index` in `state.json`. Starting a VM whose TAP device name already exists fails instead of deleting the existing device.

### Host firewall rules

fcvm never changes the host's `FORWARD` policy. Rules go in a dedicated `FCVM` chain, jumped from `FORWARD`:

| Rule | Purpose |
|------|---------|
| `FCVM -i fcvm-tap-N -o <egress> -j ACCEPT` | guest egress |
| `FCVM -o fcvm-tap-N -i <egress> -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT` | return traffic |
| `nat POSTROUTING -s <guest /30> -o <egress> -j MASQUERADE` | per-VM NAT |

Teardown deletes exactly these three. When the last VM goes away, the `FCVM` chain is removed and `ip_forward` is restored to the value it had before fcvm first enabled it (saved in `<state-dir>/.ip_forward.orig`).

The egress interface comes from parsing `ip -j route list default` as JSON; if there is no default route, `start` fails rather than guessing `eth0`.

## CNI mode

When `network.cni-network` is set (for example `fcnet`):

1. Skip TAP setup and rootfs network patching.
2. firecracker-go-sdk creates `/var/run/netns/<VMID>`, runs CNI ADD, and passes `--netns` to the jailer. Interface names: host `veth0`, guest `eth0`.
3. Guest IP/gateway/MAC come from the CNI result. **`tc-redirect-tap` is required.**
4. NFS exports are created after the address is known, so they can be scoped to the guest, and use the CNI gateway as the NFS server address.
5. On stop/cleanup, fcvm runs CNI DEL and removes the netns (no TAP teardown).

`cni-network` must match the `name` field in the CNI conflist.

### Host prerequisites

Plugins under `/opt/cni/bin`:

- `ptp`
- `host-local`
- `firewall`
- `tc-redirect-tap`

Example conflist at `/etc/cni/conf.d/fcnet.conflist`:

```json
{
  "name": "fcnet",
  "cniVersion": "0.3.1",
  "plugins": [
    {
      "type": "ptp",
      "ipMasq": true,
      "ipam": {
        "type": "host-local",
        "subnet": "192.168.127.0/24",
        "resolvConf": "/etc/resolv.conf"
      }
    },
    { "type": "firewall" },
    { "type": "tc-redirect-tap" }
  ]
}
```

```bash
sudo ./fcvm start myvm --cni-network fcnet
```

## Guest boot networking

`/usr/local/bin/fcvm-start.sh` (injected into the rootfs):

1. Sources `/etc/fcvm/network` when present (TAP mode).
2. If the interface has no global IPv4 yet, brings it up, adds the `/30`, and adds a default route via the gateway.
3. Adds a route to MMDS (`169.254.169.254`) on the guest iface.
4. Writes `FCVM_NAMESERVERS` into `/etc/resolv.conf`, but **only** if that file is missing or fcvm wrote it (it is marked with a `# managed by fcvm` first line). A resolv.conf shipped in your image is left alone.
5. Applies the mount table in `/etc/fcvm/mounts`.

DNS defaults to the host's own non-loopback resolvers from `/etc/resolv.conf`; override with `network.nameservers` or `--nameservers`.

## Host mounts (NFS and block)

Syntax: `--mount host:guest[:opt,opt...]` where an option is `ro`, `rw`, `method=nfs|block|auto`, or `size=N`. Unknown options are rejected — a typo like `:readonly` is an error, not a silent read-write mount.

**NFS (default).** Bind-mounts the host path under `<state-dir>/exports/<id>-<n>/share`, writes `/etc/exports.d/fcvm-<id>-<n>.exports`, and runs `exportfs -ra`. The export is scoped to the **guest IP only**:

```
<state-dir>/exports/vm-0/share 172.16.0.2(rw,sync,no_subtree_check,all_squash,anonuid=1000,anongid=1000)
```

If NFS setup fails, `start` fails with an error naming `method=block`. It does **not** silently fall back, because the block path is a copy and would discard the guest's writes.

**Block (`method=block`).** Copies the host directory into an ext4 image attached as `/dev/vdb`, `/dev/vdc`, … The image is sized from the source tree unless `size=` is given. This is a copy, not a live mount:

- writes are mirrored back to the host directory on `stop` and `cleanup`;
- a `ro` mount is attached read-only and is not synced back;
- files deleted in the guest are deleted on the host (mirror semantics);
- the VM crashing without a `stop`/`cleanup` leaves the writes in the image until you run `cleanup`.

## Jailer credentials and multi-VM isolation

By default all VMs share `jailer.uid` / `jailer.gid` (`1000`). Set `jailer.per-vm-uids: true` to use `uid+index` / `gid+index`. Ensure those UIDs/GIDs exist on the host; shared UIDs weaken isolation between VMs.

## Related docs

- [architecture.md](architecture.md) — lifecycle and state
- [configuration.md](configuration.md) — network config keys
- [cli.md](cli.md) — `start` / `stop` / `cleanup`
- [debug.md](debug.md) — inspecting TAPs, rules and exports on the host
