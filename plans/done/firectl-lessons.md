# Improve fcvm from firectl lessons

**Status: Slices 0-2 and the "separate track" items are DONE (verified against `main` on 2026-09-14). Slice 3 (optional Firecracker features) and Slice 4 (graceful stop) were split out into their own active plans — see [plans/optional-firecracker-features.md](../optional-firecracker-features.md) and [plans/graceful-stop.md](../graceful-stop.md).** This doc is kept as the historical design record for the completed slices.

Source: comparative reading of cloned `firectl/` (`firecracker-microvm/firectl`) vs this repo.

## Goal (met for Slices 0-2)

Steal a small set of Firecracker knobs and one testing pattern from firectl, without reshaping fcvm into a single-shot launcher. Multi-VM lifecycle, always-on jailer, TAP/NAT, assets, MMDS env/mounts, and SSH were all kept as-is.

## What shipped

### Slice 0 — Pure `firecracker.Config` builder — DONE

- `vm/fc_config.go` exists; `buildFirecrackerConfig(cfg config.Config, in machineBuildInput) firecracker.Config` at `vm/fc_config.go:41`.
- `vm/fc_config_test.go` — table-driven, root/KVM-free tests (`TestBuildFirecrackerConfigDefaults`, `TestBuildFirecrackerConfigVsockEnabled`, `TestBuildFirecrackerConfigOverrides`, `TestBuildFirecrackerConfigCNI`, `TestJailerCredsPerVM`).
- `Manager.Start` calls `buildFirecrackerConfig` (`vm/manager.go:223`) instead of inlining config construction.

### Slice 1 — Machine knobs — DONE

- `KernelArgs`, `LogLevel`, `CPUTemplate`, `DisableSMT` config fields in `config/config.go:64,66-68`, with `Validate()` allowlisting known CPU templates (`config.go:169,192-196`) and matching defaults in `Default()`.
- CLI flags + viper bindings in `cmd/root.go:47-51,68-72`, defaults at `:116,118-120`.
- Builder wiring in `vm/fc_config.go:42-50,74-79,85-86`.
- `cmd/version.go:12-20` prints the supported Firecracker version alongside the app version.

### Slice 2 — Jailer phase 1 — DONE

Same as [plans/jailer-isolation.md](../jailer-isolation.md) Phase 1 (numa-node, daemonize, parent-cgroup, cgroup) — implemented via the same builder, covered by `fc_config_test.go:101-153`.

### Separate track — DONE

1. Mounted folder emptied on microVM crash/`fcvm cleanup` — fixed: `cleanupVM` calls `syncBlockMounts` before `teardownState` (`vm/manager.go:524-528`).
2. Sync block-fallback images back to host directory on stop — fixed: `syncBlockMounts`/write-back helpers (`vm/manager.go:551-559` onward); see [plans/done/mount-writeback.md](mount-writeback.md) for full detail.

## What's still open

Split into their own plans (not implemented as of 2026-09-14):

- **Slice 3** — optional Firecracker features: initrd, general vsock device config, metrics FIFO, user extra drives, root partuuid. See [plans/optional-firecracker-features.md](../optional-firecracker-features.md).
- **Slice 4** — graceful Firecracker API shutdown before SIGTERM/SIGKILL on stop. See [plans/graceful-stop.md](../graceful-stop.md).

## Non-goals (still locked)

- Jailer is not optional.
- TAP+MASQUERADE was not replaced with "bring your own TAP" as the only path.
- CLI was not switched to go-flags or flattened into `package main`.
- Structured MMDS (`env` / `mounts`) was not replaced with raw `--metadata` JSON.
- No multi-NIC TAP list.

## Explicitly rejected copies from firectl (still rejected)

- Optional non-jailer `VMCommandBuilder` path.
- Pre-created TAP as default networking.
- Foreground-only VMM supervisor replacing `state.json` + `waitVM`.
- Raw JSON `--metadata` replacing fcvm's MMDS schema.
- Multi-NIC TAP list without a concrete multi-homing need.
