# Version history

| Version | Date | Summary |
|---------|------|---------|
| [v1.2.0](release_notes.md) | 2026-07-31 | Correctness/security/CLI hardening; experimental vsock; `install.sh` |
| v1.1.0 | 2026-07-26 | Jailer/machine knobs; optional CNI; product docs tree |
| v1.0.0 | 2026-07-17 | Initial CLI; GoReleaser; `fcvm version` |

Full notes for the latest release: [release_notes.md](release_notes.md).

## v1.2.0 (2026-07-31)

Hardened VM index allocation, mounts, stop/list liveness, NFS scoping, and download integrity. Added experimental vsock/`vsock-exec`, guest-agent download, experimental gating, TAP subnet collision guard, and curl|bash `install.sh`. See [release_notes.md](release_notes.md).

## v1.1.0 (2026-07-26)

Exposed Firecracker machine and jailer knobs via a testable config builder; optional CNI networking and per-VM jailer UIDs. Fixed NFS cleanup wiping host `--mount` directories; removed no-op `--expose-kvm`. Rewrote product docs into `docs/`.

## v1.0.0 (2026-07-17)

First release of the fcvm CLI for jailed Firecracker microVM lifecycle (download, build-rootfs, start/stop, mounts, SSH exec). GoReleaser builds, `fcvm version`, configurable rootfs size, and initial install/kernel/rootfs documentation.
