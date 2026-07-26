# VM liveness and process handling

`stop` can kill an unrelated process, and `list` reports dead VMs as running.

## Issue 1 — `stopVMProcess` is unsafe and unconditional

### Root cause

[vm/manager.go](../vm/manager.go):

```go
proc, err := os.FindProcess(state.PID)
if err != nil {
	return
}
_ = proc.Signal(syscall.SIGTERM)
time.Sleep(2 * time.Second)
_ = proc.Kill()
```

Three defects in six lines:

- On Linux `os.FindProcess` never returns an error, so the guard is dead code and gives false reassurance.
- The `Kill` is unconditional. If the VM shut down cleanly in under two seconds, the PID may already have been recycled by the kernel, and fcvm sends SIGKILL to whatever now owns it. Running as root, that can be anything on the box.
- The wait is a flat sleep. A guest that needs three seconds to flush gets SIGKILLed; a guest that exits in 50 ms still costs two seconds per `stop`, and `cleanup --all` pays it per VM serially.

### Fix

Poll instead of sleeping, and confirm identity before signalling:

1. Verify the PID still belongs to this VM before touching it. Cheapest reliable check on Linux is reading `/proc/<pid>/stat` start-time and comparing it to a start-time recorded in `state.json` at boot; a simpler middle ground is matching the process's `/proc/<pid>/cmdline` against the jailer/firecracker binary and VM id. Record whichever field is chosen in state at start time, since fcvm already writes `state.json` right after `machine.PID()`.
2. Send SIGTERM, then poll `syscall.Kill(pid, 0)` on a short interval up to a bounded deadline.
3. Escalate to SIGKILL only if the process is still alive *and* still identifies as ours at the deadline.
4. Make the deadline configurable (`stop-timeout`, default a few seconds) rather than a magic `2 * time.Second`.

The SDK also offers `machine.Shutdown`/`StopVMM`, but `Stop` runs in a different process from the one that started the VM and only has the PID, so signalling stays the mechanism.

## Issue 2 — `list` does not check liveness

### Root cause

`listCmd` ([cmd/list.go](../cmd/list.go)) prints `ID`, `GUEST IP`, `PID`, and an uptime computed from `StartedAt` for every `state.json` it finds. A VM that crashed, or whose host rebooted, is indistinguishable from a running one, and its uptime keeps climbing.

This is not only cosmetic: those stale entries are what inflate the count in `nextVMIndex` (see [vm-index-allocation.md](vm-index-allocation.md)), so a crashed VM actively causes the index collision bug.

### Fix

Add a `STATUS` column backed by the same identity check used by `stopVMProcess` — `running` when the PID is alive and ours, `stopped` otherwise. Share one `func (s *State) IsRunning() bool` (or a free function taking a state) so `list`, `stop`, and the allocator all agree on what "live" means.

Follow-on, cheap once the check exists: `cleanup --all` can report which VMs it found dead, and the background `waitVM` goroutine — which currently only logs `VM %q exited` and lets the state file rot ([vm/manager.go](../vm/manager.go)) — can mark the state as exited so a later `list` is accurate even without a liveness probe.

## Locked decisions

| Topic | Choice |
|-------|--------|
| Identity check | PID liveness **plus** a recorded identifier from start time; never PID alone |
| Escalation | SIGTERM → bounded poll → SIGKILL only if still alive and still ours |
| Timeout | Config `stop-timeout`, default 5s |
| Shared helper | One liveness function used by `stop`, `list`, `cleanup`, and index allocation |
| Dead VMs in `list` | Shown with `stopped` status, not hidden — the user needs to know they need cleanup |
| Auto-cleanup | No. `list` stays read-only; removing state is `cleanup`'s job |

## Code touch list

| Area | Change |
|------|--------|
| [vm/state.go](../vm/state.go) | Record process identity at start; `IsRunning` helper |
| [vm/manager.go](../vm/manager.go) | Poll-based `stopVMProcess` with identity check; `waitVM` marks state exited |
| [cmd/list.go](../cmd/list.go) | `STATUS` column; write via `cmd.OutOrStdout()` (see [cli-ergonomics.md](cli-ergonomics.md)) |
| [config/config.go](../config/config.go) | `stop-timeout` |
| [docs/cli.md](../docs/cli.md) | Document the status values |

## Check to leave behind

The identity check is the risky logic, so test it without a VM: start a short-lived `os/exec` process in the test, record its identity the way `Start` would, assert `IsRunning` is true; wait for it to exit, assert `IsRunning` is false. Then assert that a state carrying a *stale* identity for a live PID (simulate by recording a deliberately wrong identifier for the test process) reports not-running — that is the case that prevents killing an innocent process, and it is the whole point of the change.

## Non-goals

- No supervision, restart policies, or health checks.
- No daemon watching VMs; `waitVM` stays a per-process goroutine.
- Do not remove state automatically when a VM is found dead.
- No cgroup-based process tracking.

## Success criteria

- Stopping a VM that already exited does not signal anything and does not sleep.
- A recycled PID is never signalled.
- A clean guest shutdown completes in well under the old fixed two seconds.
- `fcvm list` shows `stopped` for a crashed VM, and the next `fcvm start` does not reuse a live VM's index because of it.
