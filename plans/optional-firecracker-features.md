# Optional Firecracker features (firectl parity, low priority)

Source: split out of the now-archived [plans/done/firectl-lessons.md](done/firectl-lessons.md) Slice 3. Each item is independent and optional — implement only when a real use case shows up. Prefer one PR per item.

## Current state

`vm/fc_config.go` builds `firecracker.Config` from `config.Config` + `machineBuildInput` (see `buildFirecrackerConfig`, `vm/fc_config.go:41`). None of the fields below exist on `config.Config` or the builder today (confirmed absent repo-wide as of 2026-09-14).

## 3a — Initrd support

Goal: let operators boot a rootfs that needs an initrd.

- Config: add `initrd: ""` (path) to `config.Config` (`config/config.go`), default empty, `Validate()` no-ops when empty, `requireTrustedAsset`-style check when set (mirror kernel path validation).
- CLI: flag + `BindPFlag` in `cmd/root.go` alongside existing `--kernel`/`--rootfs`.
- Builder: in `vm/fc_config.go`, add `InitrdPath` to `MachineCfg` (verify exact SDK field name against the vendored firecracker-go-sdk version) when `cfg.Initrd != ""`.
- Jailer: confirm `NaiveChrootStrategy` copies the initrd file into the jail chroot the same way it does the kernel; if not, extend the chroot strategy or copy manually before `NewMachine`.
- Tests: `vm/fc_config_test.go` case asserting `InitrdPath` set/unset; if chroot-copy logic changes, add a manager-level test or document manual verification.
- Docs: `docs/configuration.md`, `fcvm.example.yaml` commented example.

## 3b — Real vsock device configuration

Goal: let operators declare arbitrary vsock devices (`path:cid`), not just the hardcoded vsock-exec transport.

- Today: `vsock/output.go:157-169` hardcodes `GuestCID = 3`, `UDSName = "vsock.sock"`, `DeviceID = "1"` — this is the internal vsock-exec transport, not a general config surface. Do not remove or repoint it; add a *separate* mechanism alongside it.
- Config: `vsock-devices: []string` (yaml list of `path:cid`, mirroring firectl's repeatable `--vsock-device=PATH:CID` flag) in `config.Config`.
- Parser: small helper, e.g. `parseVsockDevices([]string) ([]firecracker.VsockDevice, error)`, with a sentinel error for malformed entries (bad CID, missing `:`).
- Builder: `vm/fc_config.go` appends parsed entries alongside the existing internal vsock-exec device — confirm the SDK/Firecracker API allows multiple vsock devices per VM before promising multi-device support; document the limit if it only allows one.
- Path handling: UDS paths are relative to the jailer chroot root — document this, matching how `LogPath`/`SocketPath` are already kept chroot-relative in `vm/fc_config.go`.
- Tests: parser unit tests (good/bad CID, good/bad path); builder test asserting devices land on `MachineCfg`.
- Docs: `docs/configuration.md`, `fcvm.example.yaml`.

## 3c — Metrics FIFO / richer logs

Goal: expose Firecracker's metrics FIFO output (distinct from the existing `LogPath` API log).

- Config: `metrics-fifo: ""` (path) in `config.Config`.
- Builder: set the metrics-fifo field on `firecracker.Config` when non-empty (verify exact SDK field name).
- Jailer constraint: the FIFO must be creatable/openable from inside the jail chroot — `mkfifo` before `NewMachine`, placed at a chroot-relative path, same pattern as `LogPath`.
- Tests: builder test for the field; skip an end-to-end FIFO-read test unless a use case demands it (document as manual verification instead).
- Docs: clarify the difference between `LogPath` (already surfaced via `fcvm attach`) and this metrics stream.

## 3d — User-supplied extra drives (`--add-drive` ro/rw)

Goal: let operators attach additional disk images beyond the rootfs and the existing mount-fallback block images.

- Distinct from today's `BlockDrives`: `vm/fc_config.go:24-39,61-67` and `vm/manager.go:204-226` populate `BlockDrives` *only* from the mount degrade-to-block fallback (`mount.ReadOnly()`), always tied to a `--mount` entry. This item adds an independent, operator-supplied drive list unrelated to mounts.
- CLI: repeatable `--add-drive=path[:ro|rw]` flag on `start` (`cmd/start.go`), default `rw` if suffix omitted (mirror firectl's parsing).
- Config: not necessarily persistent — start-time-only like `--mount` is fine, unless config-file parity is wanted, in which case add `extra-drives: []string`.
- Builder: append entries to the `Drives` slice in `vm/fc_config.go`, honoring `IsReadOnly` on each (same test pattern already used for the mount-fallback block drive at `fc_config_test.go:143-152`).
- Trust/ownership: apply the same `requireTrustedAsset`/chown rules used for the rootfs copy, since these images also need to be readable inside the jail chroot.
- Tests: builder test with a mix of ro/rw extra drives; CLI flag-parsing test.
- Docs: `docs/cli.md`, `docs/configuration.md`.

## 3e — Root Partuuid

Goal: support booting a rootfs image that is a partition table rather than a bare filesystem.

- Only worth doing if/when fcvm needs to boot a partitioned root image. No current use case.
- Config: `root-partuuid: ""` on `config.Config`; when set, append `root=PARTUUID=<value>` to kernel args instead of the current implicit root-device reference (confirm current kernel-args assumption before implementing).
- Builder + kernel-args wiring, test, docs — same pattern as the other knobs above.
- **Recommendation: skip until a concrete rootfs layout needs it.** Track as backlog only, do not implement speculatively.

## Verification (for whichever sub-items are implemented)

- `go test ./vm/... ./config/... ./cmd/...`
- `go build -buildvcs=false -o fcvm .`
- Manual smoke test on hardware with KVM for any item that changes what gets attached to the running VM (initrd, vsock, extra drives).

## Non-goals

- Do not implement all five sub-items in one PR — one feature per PR, only when needed.
- Do not touch the existing vsock-exec transport (`vsock/output.go`) while adding 3b's general vsock config.
- Do not change the mount-fallback `BlockDrives` mechanism while adding 3d's extra-drives feature — keep them separate code paths even if they share the `Drives` slice in the final config.
