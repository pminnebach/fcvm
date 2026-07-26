# Serial UART and Firecracker log bounding

Close the two host DoS surfaces Firecracker calls out in [prod-host-setup.md](https://github.com/firecracker-microvm/firecracker/blob/main/docs/prod-host-setup.md): guest-writable UART flood, and unbounded `firecracker.log` growth. Defaults stay debug-friendly; production knobs are opt-in.

## Goal

- Config knobs to disable guest UART (`8250.nr_uarts=0`) for production profiles.
- Bound Firecracker API logs (named pipe or rotatable file under the state/jail dir) so a noisy VM cannot fill the host disk by default once implemented.
- Document the mutual exclusion with interactive serial console ([serial-console.md](serial-console.md)).

## Symptom

Today every VM boots with serial console enabled and writes an unbounded host-side log:

```go
// vm/fc_config.go
defaultKernelArgs = "console=ttyS0 reboot=k panic=1 net.ifnames=0 biosdevname=0"
…
LogPath: fcLogName, // "firecracker.log" inside the jail chroot
```

A compromised or runaway guest can spam UART; Firecracker mirrors that to the host process stdio/log path. Separately, `firecracker.log` grows without rotation for the life of the VM (and often longer if leftover under the chroot).

## Root cause

fcvm optimizes for SSH/debug ergonomics: kernel args always include `console=ttyS0`, and the SDK `LogPath` is a plain file with no size cap or logrotate integration. There is no production profile that turns UART off or redirects logs to a bounded sink.

## Locked decisions

| Topic | Choice |
|-------|--------|
| UART disable | Expose a config/flag that appends (or switches to) `8250.nr_uarts=0` and drops/omits `console=ttyS0` when production hardening is on |
| Default | Stay console-on (today’s SSH/debug workflow). Document that production should turn UART off |
| Config shape | Prefer an explicit knob (e.g. `disable-uart: true` or `production: true` that implies it) over forcing operators to hand-edit full `kernel-args` |
| Firecracker logs | Prefer named pipe (`LogFifo`) or a file under state/jail dir with documented logrotate; once implemented, do not leave an unbounded grow-forever default for new installs |
| Conflict with console | Interactive console ([serial-console.md](serial-console.md)) **requires** UART. Production profile that disables UART and console mode are mutually exclusive; console plan remains for debug hosts |
| Daemonize | Unchanged by this plan; see serial-console for screen/`daemonize` interaction |

## Fix

1. Add a config field (and matching CLI flag / viper bind) for UART disable. When set, build kernel args without `console=ttyS0` and with `8250.nr_uarts=0` (merge carefully with user-supplied `kernel-args`).
2. Document in README/configuration that production hosts should set the knob; keep default `false`.
3. Change log sink wiring in [`vm/fc_config.go`](../vm/fc_config.go) / [`vm/manager.go`](../vm/manager.go): use SDK `LogFifo` (or equivalent) under the jail/state path, or a file path that logrotate can truncate/rotate.
4. Ship a short logrotate snippet or docs section pointing at the log path under the chroot/state dir.
5. In docs and in [`plans/serial-console.md`](serial-console.md) cross-link: if UART is disabled, `fcvm console` must fail with a clear error (when console exists) / production mode must not be combined with console-on start.
6. Example yaml: comment a “production” block showing UART off + log path notes.

## Code touch list

| Area | Change |
|------|--------|
| [`config/config.go`](../config/config.go) | New field + defaults |
| [`cmd/root.go`](../cmd/root.go) | Flag / bind / help text |
| [`vm/fc_config.go`](../vm/fc_config.go) | Kernel-arg merge; LogPath vs LogFifo |
| [`vm/manager.go`](../vm/manager.go) | Ensure fifo/file exists with correct ownership under jail |
| [`fcvm.example.yaml`](../fcvm.example.yaml) | Production comments |
| [`docs/configuration.md`](../docs/configuration.md), [`docs/kernel.md`](../docs/kernel.md) | Document knobs and console conflict |
| [`plans/serial-console.md`](serial-console.md) | Cross-link mutual exclusion (docs-only edit at implement time if still open) |

## Check to leave behind

Unit test on kernel-arg construction: with the knob off, args still contain `console=ttyS0` and not `8250.nr_uarts=0`; with the knob on, the reverse. Pure string building — no root, no Firecracker.

Optional: assert config unmarshaling sets LogFifo/path fields when the production log mode is selected.

## Non-goals

- Do not change the default to UART-off in this feature (backward compatibility / debug workflow).
- Do not implement interactive console here (separate plan).
- Do not build a custom log daemon inside fcvm; logrotate or fifo consumers are enough.
- Do not disable MMDS or change SSH paths.

## Success criteria

- Operator can disable UART via config without hand-writing the full cmdline.
- Documented production profile turns UART off and points at a bounded/rotatable log sink.
- Docs state clearly that production UART-off and interactive serial console are mutually exclusive.
- Default `fcvm start` behavior for existing users is unchanged until they opt in.
