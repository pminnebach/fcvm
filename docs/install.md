# Install

## Quick install

```bash
curl -sSL https://raw.githubusercontent.com/pminnebach/fcvm/refs/heads/main/install.sh | sudo bash
```

Installs the latest `fcvm` release to `/usr/local/bin`, downloads Firecracker, jailer, and the default kernel into `~/.fcvm`, and prints a dependency status table. Does not build a rootfs — use `fcvm build-rootfs` after Docker is available.

## Requirements

| Requirement | Why |
|-------------|-----|
| Linux with `/dev/kvm` | Firecracker |
| Root (sudo) | Jailer, TAP/NAT, NFS, cleanup |
| Go 1.26+ (to build from source) | Module requires Go 1.26.5 |
| Host tools | `ip`, `iptables`, `cp`, Docker (for `build-rootfs`), `mkfs.ext4`, `truncate`, NFS server tools for mounts; `unsquashfs` for squashfs downloads |

## Build fcvm

Routine build (also the project verification sequence):

```bash
go test ./...
go build -buildvcs=false -o fcvm .
```

With an explicit version string:

```bash
go build -buildvcs=false \
  -ldflags="-X 'github.com/pminnebach/fcvm/cmd.version=$VERSION'" \
  -o fcvm .
```

GoReleaser (snapshot, injects short commit into `fcvm version`):

```bash
goreleaser build --snapshot --clean --single-target
# binary under dist/fcvm_linux_*/fcvm
sudo install -m 755 dist/fcvm_linux_amd64_v1/fcvm /usr/local/bin/
```

Release builds are linux/amd64, `CGO_ENABLED=0` (see `.goreleaser.yaml`). There is no project Makefile for the Go app.

## First-time assets

Default paths under `~/.fcvm` (expanded from the real user home when using sudo):

```bash
sudo ./fcvm download firecracker   # also installs jailer from the same release tarball
sudo ./fcvm download kernel
sudo ./fcvm download rootfs --url 'https://…'   # or build-rootfs
# optional (experimental; needed for --enable-vsock):
sudo ./fcvm download guest-agent --enable-experimental
# optional:
sudo ./fcvm build-rootfs --dockerfile ./Dockerfile
```

`download jailer` downloads the same release binaries; `download jailer --build` clones Firecracker and builds via `tools/devtool`.

### Integrity

The firecracker/jailer tarball is checked against the SHA-256 published with the release before anything is unpacked, and the download fails if that checksum is unavailable — these binaries are executed as root. The default `download guest-agent` path uses the same fail-closed checksum check against the fcvm release. Kernel and rootfs URLs (and `guest-agent --url`) are arbitrary, so pass `--sha256 <hex>` to verify them; without it fcvm warns. Plain `http://` URLs are refused unless you pass `--insecure`.

## Quick start

```bash
sudo ./fcvm start myvm --env FOO=bar --mount /data:/mnt/data
sudo ./fcvm list
sudo ./fcvm exec myvm -- uname -a
sudo ./fcvm shell myvm
sudo ./fcvm stop myvm
sudo ./fcvm cleanup --all
```

Config file: `~/.fcvm.yaml`, `./.fcvm.yaml`, or `--config`. See [configuration.md](configuration.md) and [fcvm.example.yaml](../fcvm.example.yaml).

Smoke test (needs root + `/dev/kvm` + assets already present):

```bash
sudo ./fcvm self-check
```

## Related docs

- [cli.md](cli.md) — full command reference
- [rootfs.md](rootfs.md) — custom images
- [kernel.md](kernel.md) — stock and custom kernels
- [network.md](network.md) — TAP and CNI host setup
