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
| `--cni-network` | (empty) | CNI network name; empty = static TAP |
| `--verbose` | `false` | Verbose logging |
| `--wait-timeout` | `120` | Seconds to wait for guest SSH |

Most operational commands require **root**.

## `fcvm start [id]`

Start a microVM via jailer. If `id` is omitted (and `--id` unset), generates `vm-<unix-timestamp>`.

| Flag | Meaning |
|------|---------|
| `--id` | VM identifier (alternative to positional arg) |
| `--mount` | `host:guest[:ro]` (repeatable); method `auto` |
| `--env` | `KEY=VALUE` (repeatable map) |

```bash
sudo ./fcvm start myvm --env FOO=bar --mount /data:/mnt/data
```

## `fcvm stop <id>`

Signal the VM and tear down TAP/CNI, NFS, jailer tree, and state.

## `fcvm list`

Tabular list: ID, guest IP, PID, uptime.

## `fcvm exec <id> -- <command…>`

Run a command in the guest over SSH.

```bash
sudo ./fcvm exec myvm -- uname -a
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
| `firecracker` | Latest GitHub release tarball → `bin/firecracker` + `jailer` |
| `jailer` | Same release, or `--build` via Firecracker `devtool` |
| `kernel` | CI latest, or `--url` / `kernel-url` |
| `rootfs` | `--url` required |

## `fcvm build-rootfs`

Build an ext4 image from a Dockerfile. See [rootfs.md](rootfs.md).

| Flag | Default |
|------|---------|
| `--dockerfile` | (required) |
| `--tag` | `fcvm-rootfs:latest` |
| `--output` | config `rootfs` |
| `--size` | `4G` |

## `fcvm self-check`

Starts and stops a VM named `selfcheck` if `/dev/kvm` exists and the process is root. Skips (exit 0) when KVM is absent.

## `fcvm version`

Prints the build-time version (GoReleaser ldflags or empty/default for plain `go build`).
