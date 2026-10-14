# Version history

| Version | Date | Summary |
|---------|------|---------|
| [v1.2.2](release_notes_v1.2.2.md) | 2026-09-14 | Fix uppercase `.fcvm.yaml` env vars getting lowercased in the guest |
| [v1.2.1](release_notes_v1.2.1.md) | 2026-09-14 | Fix `shell`/`exec` failing right after `start` with "No route to host" |
| [v1.2.0](release_notes_v1.2.0.md) | 2026-07-31 | Correctness/security/CLI hardening; experimental vsock; `install.sh` |
| v1.1.0 | 2026-07-26 | Jailer/machine knobs; optional CNI; product docs tree |
| v1.0.0 | 2026-07-17 | Initial CLI; GoReleaser; `fcvm version` |

Full notes for the latest release: [release_notes_v1.2.2.md](release_notes_v1.2.2.md).

## v1.2.2 (2026-09-14)

Env vars configured under `env:` in `.fcvm.yaml` now keep their original casing in the guest, instead of being lowercased by viper's config-file parsing. See [release_notes_v1.2.2.md](release_notes_v1.2.2.md).

## v1.2.1 (2026-09-14)

`shell` and `exec` now wait for guest SSH readiness before connecting, instead of a single unretried attempt, fixing a race where they could fail with "No route to host" immediately after `start`. See [release_notes_v1.2.1.md](release_notes_v1.2.1.md).

## v1.2.0 (2026-07-31)

Hardened VM index allocation, mounts, stop/list liveness, NFS scoping, and download integrity. Added experimental vsock/`vsock-exec`, guest-agent download, experimental gating, TAP subnet collision guard, and curl|bash `install.sh`. See [release_notes_v1.2.0.md](release_notes_v1.2.0.md).

## v1.1.0 (2026-07-26)

Exposed Firecracker machine and jailer knobs via a testable config builder; optional CNI networking and per-VM jailer UIDs. Fixed NFS cleanup wiping host `--mount` directories; removed no-op `--expose-kvm`. Rewrote product docs into `docs/`.

## v1.0.0 (2026-07-17)

First release of the fcvm CLI for jailed Firecracker microVM lifecycle (download, build-rootfs, start/stop, mounts, SSH exec). GoReleaser builds, `fcvm version`, configurable rootfs size, and initial install/kernel/rootfs documentation.
