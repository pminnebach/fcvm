# Networking

fcvm supports two guest networking modes. Default is static TAP + MASQUERADE. Set `network.cni-network` (or `--cni-network`) to switch to CNI.

## Static TAP (default)

When `cni-network` is empty:

1. fcvm creates `fcvm-tap-N` and assigns a /30 from `tap-ip` / `guest-ip` (offset by VM index).
2. Shared host MASQUERADE provides outbound NAT.
3. Rootfs is patched with the guest IP; stop tears down the TAP.

## CNI mode

When `network.cni-network` is set (e.g. `fcnet`):

1. fcvm skips TAP setup and rootfs network patching.
2. firecracker-go-sdk creates `/var/run/netns/<VMID>`, runs CNI ADD, and passes `--netns` to the jailer.
3. Guest IP/gateway come from the CNI result (`tc-redirect-tap` required).
4. NFS mount metadata uses the CNI gateway as the NFS server address.
5. On stop/cleanup, fcvm runs CNI DEL and removes the netns (it does **not** call TAP teardown).

`cni-network` must match the `name` field in your CNI conflist.

### Host prerequisites

Plugins under `/opt/cni/bin`:

- `ptp`
- `host-local`
- `firewall`
- `tc-redirect-tap`

Config under `/etc/cni/conf.d`, for example `fcnet.conflist`:

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

Example:

```bash
sudo ./fcvm start myvm --cni-network fcnet
```

## Jailer credentials

By default all VMs share `jailer.uid` / `jailer.gid`. Set `jailer.per-vm-uids: true` to use `uid+index` / `gid+index` (Firecracker prod-host-setup recommendation). Ensure those UIDs/GIDs exist on the host; shared UIDs weaken isolation between VMs.
