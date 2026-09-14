# Code review backlog: usability and bug fixes

Source: [docs/review.md](../docs/review.md) (a point-in-time review report, left unedited as the original record). This plan tracks the concrete fixes still outstanding as of 2026-09-14.

Each item below is independent and small — safe to do as separate PRs/commits, in any order. Priority follows `docs/review.md`'s ordering.

## 1. Spurious SSH host-key warning on every `exec`/`shell`/mounted `start`

**File:** `guest/ssh.go`, `sshOpts` (around line 64).
**Problem:** `StrictHostKeyChecking=no` + `UserKnownHostsFile=/dev/null` without `-o LogLevel=ERROR` makes OpenSSH print `Warning: Permanently added '<ip>' ...` to stderr on every connection, since nothing is ever actually remembered between runs.
**Fix:** add `-o LogLevel=ERROR` (or `=QUIET`) to the `sshOpts` slice.
**Verify:** run `fcvm exec <id> true` and confirm stderr is empty; extend a unit test asserting `LogLevel=ERROR` is present in the constructed args if `sshOpts` is unit-testable in isolation.

## 2. Shell completion breaks on an unrelated config error

**File:** `cmd/root.go`, `PersistentPreRunE` (around lines 89-93).
**Problem:** the skip-list checks `cmd.Name()` against `"completion"`, but cobra's generated completion command is a parent group — the leaf command that actually runs is `bash`/`zsh`/`fish`/`powershell`/`__complete`, none of which ever equal `"completion"`. A bad config value therefore breaks tab-completion entirely.
**Fix:** walk up to the root of the completion subtree instead of comparing only the leaf name, e.g.:
```go
for c := cmd; c != nil; c = c.Parent() {
    if c.Name() == "completion" {
        return nil
    }
}
if cmd.Name() == cobra.ShellCompRequestCmd || cmd.Name() == cobra.ShellCompNoDescRequestCmd {
    return nil
}
```
(Check exact cobra constant names against the vendored version.) Keep the existing `"experimental", "version", "help"` cases as-is.
**Verify:** reproduce with a malformed `~/.fcvm.yaml` (e.g. `vcpu-count: "not-a-number"`) and confirm `./fcvm --config <bad file> completion bash` now succeeds; add a `cmd/root_test.go` case invoking `PersistentPreRunE` against a `completion`-tree command with a broken config loader stubbed in.

## 3. CNI mode: NFS export silently mis-scoped when gateway resolves empty

**File:** `vm/manager.go`, `setupMounts` (around lines 379-382), and `resolveCNIAddrs` (around lines 410-428).
**Problem:** `resolveCNIAddrs` only errors when the CNI result has no IP at all; when `ipCfg.Gateway == nil` it returns an empty `gateway` string. `setupMounts` then falls back to `server := guestIP`, producing an NFS source of `<guestIP>:<path>` — the guest is told to mount an export from itself, which can never work, and the failure is silent.
**Fix:** treat an empty CNI gateway as a hard error in `setupMounts` (or in `resolveCNIAddrs`) whenever mounts are actually configured for the VM, e.g.:
```go
if useCNI && gateway == "" && len(pending) > 0 {
    return fmt.Errorf("cni: no gateway resolved for %s; cannot set up NFS mounts", id)
}
```
Static TAP mode is unaffected (`gateway` is always `tapIP` there) — scope the check to the CNI branch only.
**Verify:** add a `vm` package unit test calling `setupMounts` (or the smallest testable unit around it) with an empty gateway and a non-empty mount list, asserting an error instead of a guest-side broken mount; confirm the zero-mounts case is unaffected.

## 4. Dead `--` handling in `exec`/`vsock-exec`

**Files:** `cmd/exec.go` (around lines 23-25), `cmd/vsock_exec.go` (around lines 26-28).
**Problem:** `if cmdArgs[0] == "--" { cmdArgs = cmdArgs[1:] }` is unreachable — cobra/pflag already consume a literal `--` separator before `RunE` sees `args`. The branch is harmless but misleads readers into thinking `--` needs manual handling here.
**Fix:** delete the dead branch. Optionally add a one-line comment where `args` is first used, noting that `fcvm exec <id> -- <cmd>` requires the literal `--` whenever `<cmd>` starts with something that looks like a flag, and that pflag consumes it upstream.
**Verify:** `go build ./...`; run `fcvm exec <id> -- -a` (or equivalent) and confirm behavior is unchanged (the branch was dead, so removing it changes nothing observable).

## 5. Concurrent `vsock-exec` against the same VM can steal each other's output socket

**File:** `guest/vsock.go`, `ListenOutput` (around lines 42-58).
**Problem:** every invocation unconditionally `os.Remove`s and re-`net.Listen`s the same per-VM UDS path. Two overlapping `vsock-exec` calls against the same VM race: the second yanks the path out from under the first, which then hangs on `Accept()` forever (or until context timeout).
**Fix:** either (a) take a per-VM file lock (e.g. `flock` on a sidecar lock file next to the UDS path) around the listen-through-accept-through-close window in `VsockExec`, returning a clear "vsock-exec already running for this VM" error if the lock is held; or (b) suffix the UDS path with the caller's PID and route based on connection metadata (more invasive — prefer (a) unless true concurrency is a real requirement).
**Verify:** unit test that starts two `ListenOutput` calls back-to-back against the same path and confirms the second either blocks/errors cleanly (rather than silently displacing the first) once the lock is added; manual test running two `fcvm vsock-exec` invocations against the same VM concurrently.

## 6. Leftover debug/telemetry instrumentation in `files/install-kasm.sh`

**File:** `files/install-kasm.sh`, `_debug_log` function (around lines 36-46) and its ~8 call sites.
**Problem:** hardcoded `sessionId`/`hypothesisId`/`runId` JSON debug lines appended to `/tmp/debug-ab6709.log` on every real run — leftover AI-assisted-debugging instrumentation that shipped by mistake.
**Fix:** delete the `_debug_log` function and every call site (`grep -n _debug_log files/install-kasm.sh` to enumerate all of them first).
**Verify:** `grep -c _debug_log files/install-kasm.sh` returns 0; shellcheck/run the script to confirm no dangling references remain.

## 7. Long-running steps give no progress feedback

**Files:** `rootfs/docker.go` (around line 13, `docker build ...).CombinedOutput()`), `rootfs/ext4.go` (around lines 60-64, `truncate`/`mkfs.ext4`), `assets/download.go` (`DownloadFile`).
**Problem:** all of these buffer output and print nothing until completion — `fcvm build-rootfs` can take minutes with zero CLI feedback, indistinguishable from a hang.
**Fix:**
- `rootfs/docker.go`: replace `.CombinedOutput()` with `cmd.Stdout = os.Stdout; cmd.Stderr = os.Stderr` (or route through `cmd.OutOrStdout()` per the existing CLI-ergonomics convention) so `docker build` output streams live.
- `rootfs/ext4.go`: same pattern for `truncate`/`mkfs.ext4`, or at minimum print a "creating ext4 image..." / "done" message around the call if streaming raw mkfs output is too noisy.
- `assets/download.go`: wrap `io.Copy(io.MultiWriter(f, hasher), resp.Body)` with a periodic byte-count progress line (print every N MB or every 1-2s via a ticking `io.Writer` wrapper); check existing patterns in `cmd/download.go` for how output is currently surfaced before deciding on format.
**Verify:** manual `fcvm build-rootfs` / `fcvm download kernel` run, confirm visible progress instead of silence; `go test ./rootfs/... ./assets/...` to confirm streaming doesn't break existing error-handling paths.

## Minor / documentation notes (lower priority, from `docs/review.md`'s "Minor" section)

- `docs/cli.md`'s `--mount` help text (`cmd/start.go` around line 52) omits `auto` from the listed methods even though it's the default — add it to the help string.
- `docs/cli.md` singles out `fcvm cleanup` as *the* crash-recovery command, but `fcvm stop <id>` on an already-dead VM does the same teardown — soften the docs wording to mention both.
- `SetupNFSExport` (`network/nfs.go` around line 33) resolves a relative `--mount host:...` path against the current working directory of the `fcvm start` invocation — undocumented; add a line to `docs/network.md`/`docs/cli.md`.

## Non-goals

- Do not use this plan to re-litigate `docs/review.md` item 4 (undocumented base-image dependency) — that was already fixed by removing the two-step Dockerfile chain; the one remaining loose end (`docs/rootfs.md` still references the deleted files) is tracked in [plans/repo-hygiene-followup.md](repo-hygiene-followup.md).
- No behavior changes beyond what's described per item — do not bundle unrelated refactors into these fixes.
