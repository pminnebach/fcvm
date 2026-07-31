# CLI reference

Global persistent flags (also bindable via config / `FCVM_*` env — see [configuration.md](configuration.md)):

| Flag | Default | Meaning |
|------|---------|---------|
| `--config` | `$HOME/.fcvm.yaml` / `./.fcvm.yaml` | Config file path |
| `--state-dir` | `~/.fcvm` | State directory |
| `--firecracker-bin` | `~/.fcvm/bin/firecracker` | Firecracker binary |
| `--jailer-bin` | `~/.fcvm/bin/jailer` | Jailer binary |
| `--kernel` | `~/.fcvm/images/vmlinux` | Kernel image |
| `--kernel-args` | `console=ttyS0 reboot=k panic=1 net.ifnames=0 biosdevname=0` | Kernel cmdline |
| `--rootfs` | `~/.fcvm/images/rootfs.ext4` | Template rootfs |
| `--log-level` | `Info` | Firecracker log level |
| `--cpu-template` | (empty) | e.g. `C3`, `T2`, `None` |
| `--disable-smt` | `false` | Disable SMT in guest |
| `--vcpu-count` | `2` | vCPUs |
| `--mem-size-mib` | `512` | Memory MiB |
| `--ssh-key` | `~/.fcvm/id_ed25519` | SSH private key |
| `--enable-vsock` | `false` | Attach virtio-vsock and inject the guest agent |
| `--guest-agent-bin` | `~/.fcvm/bin/fcvm-guest-agent` | Guest vsock agent binary (used when `--enable-vsock`) |
| `--cni-network` | (empty) | CNI network name; empty = static TAP |
| `--nameservers` | host resolvers | Guest DNS servers |
| `--verbose` | `false` | Verbose logging |
| `--wait-timeout` | `120` | Seconds to wait for guest SSH |
| `--stop-timeout` | `5` | Seconds to wait for a VM to exit before SIGKILL |

Most operational commands require **root**. Runtime errors print a single line; usage is shown only for flag and argument mistakes. Ctrl+C cancels an in-progress command, including the SSH wait during `start` and any download.

VM ids must be a single path component of letters, digits, `-`, `_` and `.`, at most 64 characters.

## `fcvm start [id]`

Start a microVM via jailer. If `id` is omitted (and `--id` unset), generates `vm-<unix-timestamp>`.

| Flag | Meaning |
|------|---------|
| `--id` | VM identifier (alternative to positional arg) |
| `--mount` | `host:guest[:opt,opt...]` (repeatable), see below |
| `--env` | `KEY=VALUE` (repeatable map) |

Mount options are comma-separated: `ro`, `rw` (default), `method=nfs|block|auto` (default `auto`, which means NFS), and `size=N` for block images. Unknown options are rejected rather than silently ignored.

```bash
sudo ./fcvm start myvm --env FOO=bar --mount /data:/mnt/data
sudo ./fcvm start myvm --mount /data:/mnt/data:ro
sudo ./fcvm start myvm --mount /data:/mnt/data:method=block,size=2G
```

If NFS is unavailable, `start` fails rather than silently copying the directory into the VM. See [network.md](network.md#host-mounts-nfs-and-block).

## `fcvm stop <id>`

SIGTERM the VM, wait up to `--stop-timeout` for it to exit, then SIGKILL. Mirrors writable block mounts back to their host directories, then tears down TAP/CNI, NFS, jailer tree, and state.

## `fcvm list`

Tabular list: ID, status, guest IP, PID, uptime. Status is `running` or `stopped`, determined from the process rather than from the state file existing, so a crashed VM shows as `stopped` (run `cleanup` to reclaim it).

## `fcvm exec <id> -- <command…>`

Run a command in the guest over SSH.

```bash
sudo ./fcvm exec myvm -- uname -a
```

## `fcvm vsock-exec <id> -- <command…>`

Run a command in the guest over vsock (split channel: command in, output back to this console). The VM must have been started with `--enable-vsock` (and a guest agent at `--guest-agent-bin`). See [network.md](network.md#vsock).

```bash
sudo ./fcvm start myvm --enable-vsock
sudo ./fcvm vsock-exec myvm -- uname -a
```

## `fcvm shell <id>`

Interactive SSH shell as configured by the injected authorized key.

## `fcvm attach <id>`

Tail the microVM serial log (`tail -f` style). Ctrl+C to exit.

## `fcvm cleanup [id]`

Tear down resources for one VM, or `--all` for every VM under the state dir. Best-effort when `state.json` is missing.

## `fcvm download`

| Subcommand | Notes |
|------------|-------|
| `firecracker` | Latest GitHub release tarball → `bin/firecracker` + `jailer`; SHA-256 verified against the checksum published with the release |
| `jailer` | Same release, or `--build` via Firecracker `devtool` |
| `kernel` | CI latest, or `--url` / `kernel-url` |
| `rootfs` | `--url` required; `--size` sets the ext4 size when converting a squashfs |

`kernel` and `rootfs` accept `--sha256 <hex>` to verify the download, and `--insecure` to allow a plain `http://` URL. Without a checksum they warn; a mismatch is a hard failure and leaves nothing on disk. The firecracker release is never installed unverified.

## `fcvm build-rootfs`

Build an ext4 image from a Dockerfile. See [rootfs.md](rootfs.md).

| Flag | Default |
|------|---------|
| `--dockerfile` | (required) |
| `--tag` | `fcvm-rootfs:latest` |
| `--output` | config `rootfs` |
| `--size` | `4G` (empty sizes from the image contents) |

## `fcvm self-check`

Starts and stops a VM named `selfcheck` if `/dev/kvm` exists and the process is root. Skips (exit 0) when KVM is absent.

## `fcvm version`

Prints the build-time version (GoReleaser ldflags or empty/default for plain `go build`).
