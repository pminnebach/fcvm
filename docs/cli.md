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
| `--enable-experimental` | `false` | Skip confirmation for experimental commands/flags |
| `--enable-vsock` | `false` | Attach virtio-vsock and inject the guest agent (**experimental**) |
| `--guest-agent-bin` | `~/.fcvm/bin/fcvm-guest-agent` | Guest vsock agent binary (used when `--enable-vsock`) (**experimental**) |
| `--cni-network` | (empty) | CNI network name; empty = static TAP (**experimental**) |
| `--nameservers` | host resolvers | Guest DNS servers |
| `--verbose` | `false` | Verbose logging |
| `--wait-timeout` | `120` | Seconds to wait for guest SSH |
| `--stop-timeout` | `5` | Seconds to wait for a VM to exit before SIGKILL |

Most operational commands require **root**. Runtime errors print a single line; usage is shown only for flag and argument mistakes. Ctrl+C cancels an in-progress command, including the SSH wait during `start` and any download.

Using an experimental command or flag prompts for confirmation unless you pass `--enable-experimental` (or set `enable-experimental` / `FCVM_ENABLE_EXPERIMENTAL`). See `fcvm experimental` for the full list. Non-interactive runs must use the bypass flag.

VM ids must be a single path component of letters, digits, `-`, `_` and `.`, at most 64 characters.

## `fcvm start [id]`

Start a microVM via jailer. If `id` is omitted (and `--id` unset), generates `vm-<unix-timestamp>`.

If a VM with that id already exists and is **stopped** (you stopped it) or **crashed**, `start` boots it again from its preserved disk with the configuration it was created with. Nothing is copied or rebuilt, so files and packages inside the guest are still there. A crashed VM's leftover TAP device, rules, exports and jail are torn down first. `--mount` and `--env` are rejected for an existing VM, because its shape is fixed at creation: `fcvm delete <id>` and start it again to change them. Starting a running VM, or a `legacy` VM (see `list`), is an error.

This is a cold boot: processes that were running are not restored.

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

SIGTERM the VM, wait up to `--stop-timeout` for it to exit, then SIGKILL. Mirrors writable block mounts back to their host directories, then tears down TAP/CNI, NFS and the jailer tree. **The VM's disk, configuration and network slot are kept**, so `fcvm start <id>` brings it back. Use `fcvm delete` to remove it.

Stopping an already stopped VM succeeds without doing anything. Stopping a crashed VM tears down what the crash left behind and records the stop. If the VM's process is still alive after SIGKILL and another `--stop-timeout`, `stop` fails and leaves the VM exactly as it was: nothing is synced, torn down or deleted under a process that may still hold the disk.

A crashed VM's writable block mount images are copied back to the host only if `e2fsck -fn` reports them clean, since the copy deletes host files that are missing from the image. Otherwise the host directory is left alone, a warning says so, and the image keeps the guest's writes for the next `start`. A `legacy` VM (created before VMs could be restarted) or an `incomplete` one has nothing to come back to, so `stop` deletes it.

## `fcvm restart <id>`

Stop the VM if it is running, then start it again from its disk. On a stopped or crashed VM it just starts it. An unknown id is an error: `restart` never creates a VM. It takes no `--mount` or `--env`.

## `fcvm delete <id>`

Remove a VM for good: its disk, block mount images, configuration and network slot (the slot can then go to a new VM). A running VM is refused unless you pass `--force`, which stops it first. If the VM crashed and its block images fail the check described under `stop`, they are not copied back and the guest's writes in them are lost with the VM; the warning says so.

## `fcvm list`

Tabular list: ID, status, guest IP, PID, uptime, stopped.

| Status | Meaning |
|--------|---------|
| `running` | The recorded process is alive and is still this VM |
| `stopped` | You stopped it; `STOPPED` shows how long ago. `fcvm start <id>` boots it again |
| `crashed` | The process is gone but nobody stopped it. `fcvm start <id>` recovers it |
| `legacy` | Created before VMs could be restarted; it cannot be started again. `fcvm cleanup <id>` removes it |
| `incomplete` | Creation never finished, so the disk may be half built. It is never booted; `fcvm delete <id>` clears it |

Status comes from comparing the records on disk with the real process, so it reflects what happened, not only what was last written. A stopped VM shows its guest IP in TAP mode, where the address is stable; in CNI mode the address is assigned at each start, so it shows `-`.

## `fcvm exec <id> -- <command…>`

Run a command in the guest over SSH. On a VM that is not running, `exec`, `shell`, `attach` and `vsock-exec` say whether it is stopped, crashed or legacy and how to proceed.

```bash
sudo ./fcvm exec myvm -- uname -a
```

## `fcvm vsock-exec <id> -- <command…>`

Run a command in the guest over vsock (split channel: command in, output back to this console). **Experimental.** The VM must have been started with `--enable-vsock` (and a guest agent at `--guest-agent-bin`). See [network.md](network.md#vsock).

```bash
sudo ./fcvm start myvm --enable-vsock --enable-experimental
sudo ./fcvm vsock-exec myvm --enable-experimental -- uname -a
```

## `fcvm experimental`

List experimental commands and flags. No confirmation prompt.

## `fcvm shell <id>`

Interactive SSH shell as configured by the injected authorized key.

## `fcvm attach <id>`

Tail the microVM serial log (`tail -f` style). Ctrl+C to exit.

## `fcvm cleanup [id]`

Remove one VM and everything it holds, or `--all` for every VM under the state dir. Best-effort when records are missing, so it also reclaims orphans. It removes the same things as `delete`, which means **`cleanup --all` deletes stopped VMs and their disks too**.

## `fcvm download`

| Subcommand | Notes |
|------------|-------|
| `firecracker` | Latest GitHub release tarball → `bin/firecracker` + `jailer`; SHA-256 verified against the checksum published with the release |
| `jailer` | Same release, or `--build` via Firecracker `devtool` |
| `kernel` | CI latest, or `--url` / `kernel-url` |
| `rootfs` | `--url` required; `--size` sets the ext4 size when converting a squashfs |
| `guest-agent` | Matching this fcvm version from GitHub releases → `guest-agent-bin` (**experimental**); optional `--url` for a direct binary |

`kernel`, `rootfs`, and `guest-agent` accept `--sha256 <hex>` to verify the download, and `--insecure` to allow a plain `http://` URL. Without a checksum they warn; a mismatch is a hard failure and leaves nothing on disk. The firecracker release and the default guest-agent release are never installed unverified.

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
