# CLI ergonomics

A batch of small, independent Cobra/Viper fixes. None is individually large; together they cover cancellation, error output, argument validation, and testability.

## 1. Commands ignore their context

Every `RunE` builds its own context — `mgr.Start(context.Background(), id)` in [cmd/start.go](../cmd/start.go), the same in [cmd/self_check.go](../cmd/self_check.go) — so nothing is cancellable. `Start` can sit in `guest.WaitSSH` for the full `wait-timeout` (default 120s) and Ctrl+C does nothing useful; the download commands can block indefinitely on a stalled server (see [asset-integrity.md](asset-integrity.md)).

**Fix:** in `Execute` ([cmd/root.go](../cmd/root.go)), build a `signal.NotifyContext` for `os.Interrupt`/`syscall.SIGTERM` and call `rootCmd.ExecuteContext(ctx)`. Replace every `context.Background()` in `cmd/` with `cmd.Context()`. Then thread it where it actually blocks:

- `guest.WaitSSH` takes a `ctx` and selects on `ctx.Done()` between attempts instead of `time.Sleep`.
- `assets.DownloadFile` and the S3 listing calls take a `ctx`.

Cancellation during `Start` must not leave a half-created VM: `Start` already has a `failCleanup` closure, so route the cancelled case through it.

## 2. Errors print the full usage block

`rootCmd` sets neither `SilenceUsage` nor `SilenceErrors`, so a runtime failure like `fcvm must run as root (jailer, networking, and NFS require it)` is followed by the entire command usage — burying the message that matters. `Execute` also discards the error and exits 1, relying on Cobra's default printing.

**Fix:** set `SilenceUsage: true` on the root command so usage prints only for flag/argument errors, which Cobra reports before `RunE` runs. Keep Cobra's error printing (do not set `SilenceErrors`) so there is exactly one error message.

## 3. Output is not capturable

Every command writes with `fmt.Printf`/`fmt.Println` straight to `os.Stdout`, and `listCmd` builds its tabwriter on `os.Stdout` directly. Combined with the package-level `rootCmd` and the global `viper` singleton, that is why `cmd/` has one test and 22% coverage.

**Fix:** write through `cmd.OutOrStdout()` everywhere. That alone makes in-memory CLI tests possible (`rootCmd.SetOut(&buf)`, `SetArgs(...)`, `Execute()`), which is the standard Cobra testing approach. Migrating off the global `viper` singleton is a larger change — worth doing eventually so tests do not leak config between cases, but out of scope for this pass; note it and move on.

## 4. Argument and flag validation gaps

- `cleanupCmd` ([cmd/cleanup.go](../cmd/cleanup.go)) declares no `Args`, so `fcvm cleanup a b c` silently ignores `b` and `c`. Add `cobra.MaximumNArgs(1)` (also called for in [destructive-path-guards.md](destructive-path-guards.md)).
- `--dockerfile` ([cmd/build_rootfs.go](../cmd/build_rootfs.go)) and `--url` ([cmd/download.go](../cmd/download.go)) are enforced with hand-written `if x == ""` checks. `MarkFlagRequired` is built in and produces a consistent message.
- Several places use `value, _ := cmd.Flags().GetString(...)` while others check the error. Pick one: for flags the command itself declared, ignoring the error is defensible, but do it consistently.

## 5. `mountFlag` silently downgrades read-only mounts

```go
if len(parts) > 2 && parts[2] == "ro" {
	m.Mode = "ro"
} else {
	m.Mode = "rw"
}
```

`--mount /data:/mnt:readonly` — or any typo — becomes a writable mount with no warning. Extra colon-separated fields beyond the third are dropped entirely.

**Fix:** parse the trailing fields explicitly and reject anything unrecognised. This function is also where the `method=`/`size=` options from [mount-writeback.md](mount-writeback.md) land, so do both at once and settle the grammar in one place. Guard against ambiguity with absolute paths containing colons while there.

## 6. `fcvm exec` loses argument quoting

`guest.Exec` ([guest/ssh.go](../guest/ssh.go)) appends the command argv to the `ssh` argv unmodified. `ssh` joins its remaining arguments with spaces and hands the result to the remote shell, which re-splits it:

```bash
sudo fcvm exec myvm -- touch "my file"     # creates two files, "my" and "file"
```

**Fix:** shell-quote each argument before joining, so the remote shell sees the words the user typed. Alternatively dial with `golang.org/x/crypto/ssh` — already a direct dependency, used for key handling — and send an exec request with a properly quoted command. The in-process route also removes the dependency on an `ssh` binary and the `StrictHostKeyChecking=no` flag soup, but it is the bigger change; quoting fixes the correctness bug on its own.

## Locked decisions

| Topic | Choice |
|-------|--------|
| Cancellation | `signal.NotifyContext` + `ExecuteContext`; `cmd.Context()` everywhere |
| Usage on error | `SilenceUsage: true`, keep Cobra's error printing |
| Output | `cmd.OutOrStdout()`; global viper migration deferred |
| Required flags | `MarkFlagRequired`, drop manual empty checks |
| Mount grammar | Strict parse, unknown option is an error |
| Exec quoting | Shell-quote arguments; in-process SSH client is a later option |

## Code touch list

| Area | Change |
|------|--------|
| [cmd/root.go](../cmd/root.go) | `ExecuteContext` + signal handling; `SilenceUsage`; strict `mountFlag` |
| [cmd/start.go](../cmd/start.go), [cmd/self_check.go](../cmd/self_check.go) | `cmd.Context()` |
| [cmd/cleanup.go](../cmd/cleanup.go) | `Args` validator |
| [cmd/build_rootfs.go](../cmd/build_rootfs.go), [cmd/download.go](../cmd/download.go) | `MarkFlagRequired`; pass context |
| [cmd/list.go](../cmd/list.go) and other printers | `cmd.OutOrStdout()` |
| [guest/ssh.go](../guest/ssh.go) | Quote exec arguments; `ctx` in `WaitSSH` |
| [docs/cli.md](../docs/cli.md) | Mount grammar, exec quoting note |

## Check to leave behind

Two small tests, both fast and root-free:

- Table-driven `mountFlag` cases: valid `ro`, valid `method=block`, and a rejected `:readonly` typo. The typo case is the regression that matters.
- An argument-quoting test for the `exec` argv builder: given `[]string{"touch", "my file"}`, assert the produced remote command round-trips as one argument. Extract the builder from `guest.Exec` so it can be called without spawning `ssh`.

## Non-goals

- Do not restructure `cmd/` into a constructor-per-command layout in this pass (that is the fix for the viper singleton, and it deserves its own change).
- No shell completion or man page generation work here.
- Do not change command names or existing flag names.

## Success criteria

- Ctrl+C during `fcvm start` aborts promptly and leaves no half-created VM.
- `fcvm stop x` as a non-root user prints one line, not a usage dump.
- `fcvm exec vm -- touch "my file"` creates one file.
- `--mount /data:/mnt:readonly` is rejected instead of silently becoming writable.
