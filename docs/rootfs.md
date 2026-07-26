# Rootfs

fcvm boots guests from an **ext4** disk image at the configured `rootfs` path (default `~/.fcvm/images/rootfs.ext4`). On each `start`, that template is copied to `~/.fcvm/vms/<id>/rootfs.ext4` and patched with SSH keys (and static network in TAP mode).

## Ways to get a rootfs

| Method | Command |
|--------|---------|
| Build from Dockerfile | `fcvm build-rootfs --dockerfile …` |
| Download URL | `fcvm download rootfs --url …` (required) |

Example Dockerfiles in the repo: `Dockerfile`, `Dockerfile.Default`, `Dockerfile.Ubuntu-2604`, `Dockerfile.Kasm`. Bases are Debian bookworm / Ubuntu — not Alpine.

Helper scripts under `files/` (`install-docker.sh`, `install-gvisor.sh`, `install-kasm.sh`) are copied into some Dockerfiles; fcvm does not run them itself.

## Build from Dockerfile

```bash
sudo ./fcvm build-rootfs \
  --dockerfile ./Dockerfile \
  --tag fcvm-rootfs:latest \
  --output ~/.fcvm/images/rootfs.ext4 \
  --size 4G
```

| Flag | Default | Meaning |
|------|---------|---------|
| `--dockerfile` | (required) | Path to Dockerfile |
| `--tag` | `fcvm-rootfs:latest` | Docker image tag |
| `--output` | config `rootfs` | Output ext4 path |
| `--size` | `4G` | Image size (`truncate` format) |

Pipeline (`rootfs.BuildFromDockerfile`):

1. `docker build -f <dockerfile> -t <tag> <dir>`
2. `docker create` → `docker export` → extract tar
3. Inject fcvm guest hooks and the host SSH public key
4. `truncate -s <size>` + `mkfs.ext4 -d <root> -F <output>`

Requires Docker, `truncate`, and `mkfs.ext4` on the host. Also creates/loads the SSH key at `ssh-key` (default `~/.fcvm/id_ed25519`).

### Dockerfile requirements

Install at least:

- `openssh-server` (guest access)
- `curl` (MMDS)
- `iproute2` (networking)
- `nfs-common` (NFS mounts)
- An init that runs systemd (`fcvm-start.service`) and/or `/etc/rc.local`

Minimal example (see repo `Dockerfile`):

```dockerfile
FROM debian:bookworm-slim

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        curl iproute2 nfs-common openssh-server systemd-sysv \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

RUN mkdir -p /run/sshd
```

fcvm injects boot hooks after export; you do not need to copy them into the Dockerfile.

## Download

```bash
sudo ./fcvm download rootfs --url 'https://example.com/rootfs.squashfs'
```

- **`.squashfs`:** download → `unsquashfs` → inject hooks → `mkfs.ext4` at the `rootfs` path. The image is sized from the unpacked contents unless `--size` is given.
- **Other:** save the file as-is. Hooks, SSH key, env and network config are applied on `start` via `assets.PatchExt4`, in a single loop mount.

`--url` is required; there is no default stock rootfs URL. Pass `--sha256` to verify the download.

## Injected guest layout

`rootfs.InjectHooks` writes:

| Path | Role |
|------|------|
| `/usr/local/bin/fcvm-mmds.sh` | MMDS token + GET helpers (for your own use) |
| `/usr/local/bin/fcvm-apply-mounts.sh` | mount everything in `/etc/fcvm/mounts` |
| `/usr/local/bin/fcvm-start.sh` | net, DNS, mounts at boot |
| `/etc/profile.d/fcvm.sh` | source env in login shells |
| `/etc/systemd/system/fcvm-start.service` + wants link | systemd oneshot |
| `/etc/rc.local` | fallback boot path |

The host writes three data files alongside them:

| Path | Written | Contents |
|------|---------|----------|
| `/etc/fcvm/env` | at patch time | `export KEY='value'`, shell-quoted so any value survives |
| `/etc/fcvm/network` | at patch time | `FCVM_GUEST_IP`, `FCVM_GATEWAY`, `FCVM_IFACE`, `FCVM_NAMESERVERS` |
| `/etc/fcvm/mounts` | after boot, over SSH | one mount per line: `method`, `source`, `guest path`, tab-separated |

The mount table arrives after boot because NFS exports cannot be scoped to the guest until its address is known. Env is baked in at patch time, so it does not depend on SSH and survives guest reboots.

No JSON is parsed inside the guest, and `jq` is not required.

## Nested / workload images

For nested KVM or heavy workloads, start from a richer Dockerfile (Docker-in-guest, QEMU, Kasm, etc.), enable the needed packages, then build with a larger `--size`. Pair with a custom KVM-enabled kernel — see [kernel.md](kernel.md).

## Related docs

- [install.md](install.md) — host tools and first-time assets
- [architecture.md](architecture.md) — per-VM copy and patch flow
- [cli.md](cli.md) — `build-rootfs` and `download rootfs`
