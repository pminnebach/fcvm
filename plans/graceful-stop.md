# Graceful Firecracker API shutdown on stop

Source: split out of the now-archived [plans/done/firectl-lessons.md](done/firectl-lessons.md) Slice 4.

## Problem

`Manager.Stop` → `stopVMProcess` (`vm/manager.go:435-476` as of 2026-09-14) goes straight to `syscall.Kill(state.PID, syscall.SIGTERM)`, then `SIGKILL` after `StopTimeoutSec`. firectl instead calls the Firecracker API's graceful shutdown before falling back to process signals. A hard SIGTERM/SIGKILL skips the guest's normal shutdown sequence (unmounting filesystems, flushing writes) that a graceful shutdown would trigger.

## Goal

Before signaling the process, attempt a graceful shutdown through the Firecracker API socket (if reachable from the host at the state-recorded socket path), then fall back to today's SIGTERM → SIGKILL if the graceful path fails, times out, or the socket isn't reachable (e.g. jailer chroot permissions, VM already dead).

## Design

1. State already records enough to locate the API socket (chroot dir / socket path pattern used elsewhere in `vm/manager.go` for `NewMachine`). Confirm the exact stored field/derivation before implementing (likely reconstructed from `ChrootBaseDir` + VM id, same as `jailerTreeDir`).
2. In `stopVMProcess`, before the `syscall.Kill(..., SIGTERM)` call:
   - Attempt a graceful shutdown against a `firecracker.Machine` reconstructed from the stored socket path (check the vendored firecracker-go-sdk's exact method name/signature — likely `Shutdown` or a `SendCtrlAltDel` action), OR issue the raw HTTP `PUT /actions {"action_type": "SendCtrlAltDel"}` call directly against the API socket if reconstructing a full `Machine` object is heavier than needed.
   - Use a short timeout (e.g. 5s, configurable via existing `StopTimeoutSec` or a new `graceful-timeout-sec`) — if it doesn't complete, proceed to the existing SIGTERM/SIGKILL path unchanged.
   - Only attempt this when the process is still alive and the socket is dial-able; any error (socket missing, permission denied, connection refused) falls straight through to the existing signal path with no behavior change from today.
3. Do not change `cleanup --all` / orphan-TAP teardown behavior — this only affects the single-VM `Stop` path where the process is confirmed running.
4. Jailer permissions: the API socket lives under the jail chroot, owned by the jailer uid/gid (`cfg.Jailer.UID`/`GID`, possibly per-VM via `PerVMUIDs` — see [plans/jailer-isolation.md](jailer-isolation.md)). Confirm the fcvm host process (usually root, since `start`/`stop` require root today) can still reach the socket; this should already work since fcvm today authors the chroot as root.

## Files to touch

- `vm/manager.go` — `stopVMProcess` (or `Stop`), add the graceful-attempt step before the signal fallback.
- Possibly `vm/state.go` if the socket path needs to be persisted explicitly rather than re-derived.
- `docs/architecture.md` — document the new shutdown sequence (graceful attempt → SIGTERM → SIGKILL).
- `docs/configuration.md` — document the new timeout knob if one is added.

## Tests

- Unit test with a fake HTTP server standing in for the Firecracker API socket, asserting: (a) a successful graceful response skips the signal path, (b) a failed/timed-out attempt falls through to signals unchanged. Use `httptest`-style setup over a Unix socket listener (`net.Listen("unix", ...)`) to simulate the real socket without needing actual Firecracker/KVM.
- Confirm existing signal-fallback tests still pass unmodified when the graceful attempt is skipped (socket absent in test fixtures).
- Manual smoke test on real hardware: `fcvm stop <id>` on a running VM, confirm guest shuts down cleanly rather than being hard-killed.

## Non-goals

- Do not remove the SIGTERM/SIGKILL fallback — it must remain the safety net.
- Do not regress `cleanup --all` orphan reclaim, which intentionally does not assume a live, reachable process.
- Do not block `Stop` indefinitely waiting for a graceful shutdown — always bound by a short timeout.

## Success criteria

- `fcvm stop <id>` on a healthy VM triggers a clean guest shutdown when the API socket is reachable.
- Behavior is unchanged (SIGTERM → SIGKILL) when the socket is unreachable or the graceful call fails/times out.
- `go test ./vm/...` covers both branches without requiring root/KVM.
