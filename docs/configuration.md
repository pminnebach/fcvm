# Configuration

## Precedence

Highest wins:

1. CLI flags
2. Environment variables (`FCVM_*`)
3. Config file: with `--config <path>` only that file is read; otherwise the Directory config (`./.fcvm.yaml`) is layered over the User config (`$HOME/.fcvm.yaml`)
4. Built-in defaults (`config.Default()`)

### Layering the Directory config

When fcvm runs in a directory that contains `.fcvm.yaml`, that **Directory config** is layered over the **User config** (`$HOME/.fcvm.yaml`), which is layered over the built-in defaults.

- A value set in the Directory config wins. A key it leaves out comes from the User config, then from the defaults.
- Maps merge per key. `env:` is merged per variable, the Directory config wins on a clash, and the original casing of each variable name is kept from both files.
- Lists, such as `network.nameservers`, are replaced whole, never appended.
- Only the exact current directory is checked, never its parents, and only the name `.fcvm.yaml`.
- When the current directory is the home directory, the file is read once, as the User config.
- If the current directory can't be determined or read, there is no Directory config and no error.
- `--config <path>` is exclusive: only that file is read, neither the Directory config nor the User config is, and a missing file is an error.

### `state-dir` is not allowed in a Directory config

A Directory config that sets `state-dir` is refused: the command fails with an error naming the file and the key. This keeps one state directory per user, so `fcvm list`, `stop` and `delete` see the same VMs from every directory, and two state directories never give out the same network slot (TAP device and IPs) to two running VMs.

`state-dir` still works from the `--state-dir` flag, `FCVM_STATE_DIR`, the User config and an explicit `--config` file. Every other key, including the binary paths, `kernel` and `rootfs`, is allowed in a Directory config.

### Trusting the Directory config

fcvm runs as root, and a config file decides which binaries it runs (`firecracker-bin`, `jailer-bin`, `guest-agent-bin`). So a Directory config is only trusted when:

- it is owned by the invoking user or by root. The invoking user is the `SUDO_UID` user under sudo, otherwise the effective uid; and
- it has no group or other write bit.

Otherwise the command fails with an error naming the file and the reason: wrong owner, or group- or world-writable. This stops `cd`-ing into someone else's checkout and running `sudo fcvm ...` from executing their binary as root. Fix it by taking ownership (`chown`) or removing the write bits (`chmod go-w .fcvm.yaml`).

The check applies only to a file found by the current-directory search. It does not apply to the User config, to `--config`, or when the current directory is home.

### Seeing which files were used

- With `--verbose`, each Config file read is logged at debug level with its role (`User config`, `Directory config` or `explicit --config`), in the order read.
- `fcvm self-check` prints a `config files:` line listing the files used, or `none`. It prints this before the `/dev/kvm` and root checks, so the line shows even when self-check skips.

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
| `enable-experimental` | `false` | Skip confirmation for experimental commands/flags |
| `enable-vsock` | `false` | Attach virtio-vsock and inject guest agent on start (**experimental**) |
| `guest-agent-bin` | `~/.fcvm/bin/fcvm-guest-agent` | Used only when `enable-vsock` is true (**experimental**) |

### Machine

| Key | Default | Notes |
|-----|---------|-------|
| `vcpu-count` | `2` | Must be ≥ 1 |
| `mem-size-mib` | `512` | Must be ≥ 1 |
| `log-level` | `Info` | Firecracker log level |
| `cpu-template` | (empty) | `C3`, `T2`, `T2S`, `T2CL`, `T2A`, `V1N1`, `None` |
| `disable-smt` | `false` | |
| `wait-timeout` | `120` | Seconds waiting for SSH |
| `stop-timeout` | `5` | Seconds to wait for exit before SIGKILL |
| `verbose` | `false` | |

### Network

| Key | Default | Notes |
|-----|---------|-------|
| `network.tap-ip` | `172.16.0.1` | Host side of /30 (TAP mode) |
| `network.guest-ip` | `172.16.0.2` | Guest side of /30 (TAP mode) |
| `network.cni-network` | (empty) | CNI conflist `name`; empty = TAP (**experimental**) |
| `network.nameservers` | host resolvers | Guest DNS; read from the host `/etc/resolv.conf` (loopback entries skipped), falling back to `8.8.8.8` |

See [network.md](network.md).

### Jailer

| Key | Default | Notes |
|-----|---------|-------|
| `jailer.chroot-base-dir` | `~/.fcvm/jailer` | |
| `jailer.uid` / `jailer.gid` | `1000` | |
| `jailer.per-vm-uids` | `false` | uid/gid = base + VM index |
| `jailer.numa-node` | `0` | |
| `jailer.daemonize` | `false` | Jailer `setsid` + stdio → `/dev/null`. fcvm tracks the VM via `firecracker.pid` under the jail root (required when daemonize is on). Leave `false` unless you want that; incompatible with the planned `fcvm console` attach path. |
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
    method: auto      # auto (= nfs), nfs, or block
    size: ""          # block only; empty sizes from the source tree
```

`method: auto` means NFS. It does **not** fall back to `block` when NFS is unavailable, because block mounts are copies and the fallback used to discard the guest's writes; ask for `block` explicitly if that is what you want. See [network.md](network.md#host-mounts-nfs-and-block).

Env values are written into the guest with shell quoting, so quotes, `$`, backticks and backslashes survive intact.

CLI `--env` and `--mount` append/override on `start`. Mount flag form: `host:guest[:opt,opt...]` where an option is `ro`, `rw`, `method=…` or `size=…`; unknown options are an error.

## Related docs

- [cli.md](cli.md) — flags and commands
- [architecture.md](architecture.md) — state layout
