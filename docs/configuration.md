# Configuration

## Precedence

Highest wins:

1. CLI flags
2. Environment variables (`FCVM_*`)
3. Config file (`--config`, else `$HOME/.fcvm.yaml` or `./.fcvm.yaml`)
4. Built-in defaults (`config.Default()`)

Hyphens and dots in keys become underscores in env names: `network.tap-ip` → `FCVM_NETWORK_TAP_IP`, `vcpu-count` → `FCVM_VCPU_COUNT`.

`~` in paths expands using `SUDO_USER`’s home when present, otherwise the current user’s home.

## Example file

See [fcvm.example.yaml](../fcvm.example.yaml). Minimal:

```yaml
firecracker-bin: ~/.fcvm/bin/firecracker
jailer-bin: ~/.fcvm/bin/jailer
jailer:
  chroot-base-dir: ~/.fcvm/jailer
  uid: 1000
  gid: 1000
kernel: ~/.fcvm/images/vmlinux
rootfs: ~/.fcvm/images/rootfs.ext4
vcpu-count: 2
mem-size-mib: 512
network:
  tap-ip: 172.16.0.1
  guest-ip: 172.16.0.2
ssh-key: ~/.fcvm/id_ed25519
wait-timeout: 120
env: {}
```

## Keys

### Paths and binaries

| Key | Default | Notes |
|-----|---------|-------|
| `state-dir` | `~/.fcvm` | VMs, images, bins, keys |
| `firecracker-bin` | `~/.fcvm/bin/firecracker` | Required on start |
| `jailer-bin` | `~/.fcvm/bin/jailer` | Required on start |
| `kernel` | `~/.fcvm/images/vmlinux` | Required on start |
| `kernel-url` | (empty) | Pin for `download kernel` |
| `kernel-args` | `console=ttyS0 reboot=k panic=1 net.ifnames=0 biosdevname=0` | Guest cmdline |
| `rootfs` | `~/.fcvm/images/rootfs.ext4` | Template ext4 |
| `ssh-key` | `~/.fcvm/id_ed25519` | Created if missing |

### Machine

| Key | Default | Notes |
|-----|---------|-------|
| `vcpu-count` | `2` | Must be ≥ 1 |
| `mem-size-mib` | `512` | Must be ≥ 1 |
| `log-level` | `Info` | Firecracker log level |
| `cpu-template` | (empty) | `C3`, `T2`, `T2S`, `T2CL`, `T2A`, `V1N1`, `None` |
| `disable-smt` | `false` | |
| `wait-timeout` | `120` | Seconds waiting for SSH |
| `verbose` | `false` | |

### Network

| Key | Default | Notes |
|-----|---------|-------|
| `network.tap-ip` | `172.16.0.1` | Host side of /30 (TAP mode) |
| `network.guest-ip` | `172.16.0.2` | Guest side of /30 (TAP mode) |
| `network.cni-network` | (empty) | CNI conflist `name`; empty = TAP |

See [network.md](network.md).

### Jailer

| Key | Default | Notes |
|-----|---------|-------|
| `jailer.chroot-base-dir` | `~/.fcvm/jailer` | |
| `jailer.uid` / `jailer.gid` | `1000` | |
| `jailer.per-vm-uids` | `false` | uid/gid = base + VM index |
| `jailer.numa-node` | `0` | |
| `jailer.daemonize` | `false` | |
| `jailer.parent-cgroup` | (empty) | |
| `jailer.cgroup` | (empty list) | e.g. `memory.max=1G` |

### Env and mounts

```yaml
env:
  FOO: bar
mounts:
  - host: /data
    guest: /mnt/data
    mode: rw          # rw or ro
    method: auto      # auto (prefer nfs), nfs, or block
```

CLI `--env` and `--mount` append/override on `start`. Mount flag form: `host:guest[:ro]` (method always `auto` from the flag).

## Related docs

- [cli.md](cli.md) — flags and commands
- [architecture.md](architecture.md) — state layout
