# Stuck-VMM overwatcher

Detect Firecracker processes whose API socket no longer responds and SIGKILL them using the existing PID + `pid_start` identity. Prefer a subcommand (or documented cron one-liner) over a long-running daemon.

## Goal

Smallest useful watchdog: `fcvm doctor` / `fcvm watch` (name locked at implement time) that scans known VMs, probes the API socket, and kills stuck VMMs that fail for N seconds. Operators can run it from cron.

## Symptom

A Firecracker VMM can hang with the guest unresponsive and the API socket wedged while the process still looks “alive” to a simple PID check. Firecracker prod-host-setup recommends an overwatcher that forcefully kills such processes. fcvm today has PID + `pid_start` liveness for `stop`/`list` ([`vm/state.go`](../vm/state.go), [vm-liveness.md](vm-liveness.md)) but nothing that probes the API or auto-kills stuck VMMs.

## Root cause

Liveness is process-identity based, not API-health based. There is no periodic probe of `firecracker.sock` (or the jailed socket path) and no kill path for “PID exists but API dead.”

## Locked decisions

| Topic | Choice |
|-------|--------|
| Shape | Subcommand (`fcvm doctor` and/or `fcvm watch`) — **not** a built-in daemon |
| Identity | Reuse `pid` + `pid_start` from state; never kill a recycled PID |
| Probe | API socket unresponsive for N seconds (configurable; sensible default e.g. 5–30s) |
| Action | SIGKILL the Firecracker/jailer PID recorded in state when probe fails past N |
| Cron | Document a one-liner for systemd timer / cron; do not require a resident process |
| Cleanup | After kill, prefer existing stop/cleanup paths so TAP/iptables/state do not linger (or document a follow-up `fcvm cleanup`) |

## Fix

1. Add a subcommand that lists VMs from state, resolves socket path (jail-aware — see existing jailer socket helpers), and performs a short API health check (e.g. connect + Firecracker GET `/` or SDK describe).
2. Track consecutive failures or elapsed unresponsive time; when over threshold, verify `pid_start` still matches, then `SIGKILL`.
3. Flags: `--timeout`, `--kill` / dry-run default for `doctor` vs watchful kill mode — pick the least surprising UX at implement time (dry-run by default is safer for `doctor`).
4. Document cron: `*/1 * * * * fcvm watch --kill` (example).
5. Reuse kill/cleanup helpers from stop where possible; do not fork a second teardown stack.

## Code touch list

| Area | Change |
|------|--------|
| [`cmd/`](../cmd/) | New `doctor` / `watch` command |
| [`vm/`](../vm/) | Socket probe helper; kill-with-pid_start guard |
| [`docs/cli.md`](../docs/cli.md) | Usage + cron example |
| Tests | Probe failure classification and “do not kill on pid_start mismatch” unit tests |

## Check to leave behind

Unit test: when `pid_start` does not match current process start time, kill is skipped. Optional: fake clock / fake dialer for “unresponsive for N seconds” threshold without a real VMM.

## Non-goals

- No always-on in-process supervisor inside `fcvm start`.
- No guest agent / vsock heartbeat (host API socket only).
- No automatic restart of killed VMs (operator or external orchestrator).
- No metrics/exporter unless already present.

## Success criteria

- Operator can run one command (manually or via cron) that kills only VMMs with mismatched API health and verified pid identity.
- Dry-run / doctor mode reports stuck VMs without killing (if that UX is chosen).
- Documented cron one-liner exists.
