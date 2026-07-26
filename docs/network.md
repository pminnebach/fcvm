# Networking

fcvm supports two guest networking modes. Default is static TAP + MASQUERADE. Set `network.cni-network` (or `--cni-network`) to use CNI.

Guest access after boot is SSH to the guest IP. There is no vsock or Linux bridge mode in fcvm today.

## Static TAP (default)

When `cni-network` is empty:

1. Allocate a VM index and derive a `/30` from `network.tap-ip` / `network.guest-ip` by shifting the **third octet** by the index (`network.SubnetForIndex`).
2. Create TAP `fcvm-tap-N`, assign the host `/30`, enable `proxy_arp` and `ip_forward`, set `FORWARD` ACCEPT, and add a shared host `MASQUERADE` on the default egress interface.
3. Derive guest MAC as `06:00:` + guest IPv4 bytes (`GuestMAC`).
4. Patch the per-VM rootfs with `/etc/fcvm/network` (`FCVM_GUEST_IP`, `FCVM_GATEWAY`, `FCVM_IFACE=eth0`).
5. Firecracker NIC uses static config: host TAP, guest `/30`, gateway = tap IP, nameserver `8.8.8.8`, MMDS allowed.
6. Stop/cleanup deletes the TAP (`ip link del`).

Defaults: `tap-ip: 172.16.0.1`, `guest-ip: 172.16.0.2` (VM index 0). Index 1 → `172.16.1.1` / `172.16.1.2`, and so on.

## CNI mode

When `network.cni-network` is set (for example `fcnet`):

1. Skip TAP setup and rootfs network patching.
2. firecracker-go-sdk creates `/var/run/netns/<VMID>`, runs CNI ADD, and passes `--netns` to the jailer. Interface names: host `veth0`, guest `eth0`.
3. Guest IP/gateway/MAC come from the CNI result. **`tc-redirect-tap` is required.**
4. NFS mount metadata uses the CNI gateway as the NFS server address.
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
4. Writes DNS: `nameserver 8.8.8.8` into `/etc/resolv.conf`.
5. Applies mounts and env from MMDS.

## Host mounts (NFS and block)

`--mount host:guest[:ro]` (method `auto` by default):

1. Prefer **NFS**: bind-mount the host path under `/tmp/fcvm-exports/<id>/…`, write `/etc/exports.d/fcvm-<id>.exports`, `exportfs -ra`. Guest mounts via NFS using host/gateway metadata from MMDS.
2. If NFS setup fails, fall back to a **virtio block** device: sync the host directory into an ext4 image attached as `/dev/vdX` and mount it in the guest.

Mount method can be forced with config (`method: nfs` or `method: block`). See [configuration.md](configuration.md).

## Jailer credentials and multi-VM isolation

By default all VMs share `jailer.uid` / `jailer.gid` (`1000`). Set `jailer.per-vm-uids: true` to use `uid+index` / `gid+index`. Ensure those UIDs/GIDs exist on the host; shared UIDs weaken isolation between VMs.

## Related docs

- [architecture.md](architecture.md) — lifecycle and state
- [configuration.md](configuration.md) — network config keys
- [cli.md](cli.md) — `start` / `stop` / `cleanup`
