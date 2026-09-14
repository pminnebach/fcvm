# In-process SSH client via `x/crypto/ssh`

## Problem

`guest/ssh.go` already imports `golang.org/x/crypto/ssh` but only uses it for ed25519 keypair generation (`LoadOrCreateKey`, `sshPublicFromPrivate`). Every actual connection (`WaitSSH`, `Exec`, `WriteFile`, `Shell`) still shells out via `exec.Command("ssh", ...)`. This has a real host dependency (the `ssh` binary must be installed) and carries the stderr-noise issue tracked in [plans/code-review-fixes.md](code-review-fixes.md) item 1; an in-process client removes the host-binary dependency and the shell-quoting surface area entirely.

## Goal

Replace the four `exec.Command("ssh", ...)` call sites with an in-process `x/crypto/ssh` client, eliminating the host `ssh` binary dependency. Preserve existing behavior and error semantics as closely as possible.

## Design

### Connection

- Build an `ssh.ClientConfig` using the existing keypair (`LoadOrCreateKey`) with `ssh.PublicKeys(signer)` auth, `HostKeyCallback: ssh.InsecureIgnoreHostKey()` (matches today's `StrictHostKeyChecking=no` + `/dev/null` known-hosts — no host key is ever actually persisted or checked today, so this is behavior-preserving, not a regression).
- `ssh.Dial("tcp", guestIP+":22", cfg)` replaces the connection step in all four functions. Decide whether to cache a `*ssh.Client` per guest IP for the lifetime of the process (if `Exec`/`Shell`/`WriteFile` are ever called back-to-back against the same VM within one `fcvm` invocation) or dial fresh each call — check current call patterns before choosing.

### `WaitSSH` (readiness probe)

- Currently `exec.CommandContext(ctx, "ssh", args...)` polled in a retry loop. Replace with a retry loop attempting `ssh.Dial` (or a raw TCP dial + minimal handshake) until it succeeds or `ctx` is done, same retry/backoff cadence as today.

### `Exec`

- Open a `*ssh.Session` via `client.NewSession()`, wire `session.Stdout`/`Stderr` to whatever the caller passes today (check current `Exec` signature), call `session.Run(cmd)` using the same command-string construction used today (the whole command still travels as one string to the remote shell, same as `ssh host cmd`, so existing argument-joining logic likely carries over as-is).
- Exit code mapping: `session.Run` returns `*ssh.ExitError` with `.ExitStatus()` on non-zero exit — map this to whatever contract `cmd/exec.go` currently expects from the `exec.Command`-based path.

### `Shell` (interactive)

- Needs a PTY: `session.RequestPty("xterm", ...)`, wire `session.Stdin`/`Stdout`/`Stderr` to the process's stdin/stdout/stderr, call `session.Shell()`, then `session.Wait()`. Local terminal raw-mode switching likely needs `golang.org/x/term` (check `go.mod` — probably already a transitive dependency via `x/crypto`). This is the trickiest part of the migration; test interactively, not just with unit tests.

### `WriteFile`

- No native "write file over SSH" in `x/crypto/ssh` alone. Either use `session.Run("cat > <path>")` with `session.Stdin` set to the file content (mirrors what the shell-based version effectively does today), or pull in `github.com/pkg/sftp` for a real SFTP-based write if the `cat >` approach proves fragile for the file sizes/content involved.

## Files touched

- `guest/ssh.go` — all four connection functions rewritten to use `ssh.Dial`/`ssh.Session` instead of `exec.Command`.
- `guest/ssh_test.go` (new or extended) — unit tests against a local in-process SSH test server (spun up on `localhost` with a throwaway host key using `x/crypto/ssh`'s server-side APIs) rather than a real guest VM, so these tests run without KVM/root.
- Callers in `cmd/exec.go`, `cmd/vsock_exec.go` (if it shares this path), `vm/manager.go` (`WaitSSH` call in `Start`) — check call signatures don't need to change; update all call sites if they do.

## Tests

- Spin up a minimal in-process `ssh.Server` that echoes commands/accepts a shell, and test `Exec`/`Shell`/`WriteFile`/`WaitSSH` against it without needing a real guest.
- Document a manual smoke-test path against a real guest for whoever has KVM hardware available, since PTY/interactive behavior is hard to fully verify without a real terminal.

## Non-goals

- Do not change the authentication model (still ed25519 keypair via `LoadOrCreateKey`, still no host-key verification — matches today).
- Do not add SFTP unless the simple `cat >` approach for `WriteFile` proves insufficient.
- Do not change any CLI-visible behavior — this is an internal transport swap.

## Success criteria

- No `exec.Command("ssh", ...)` remains in `guest/ssh.go`.
- No dependency on the host `ssh` binary for `fcvm exec`/`shell`/`start` (mounted)/readiness wait.
- `go test ./guest/...` passes against an in-process test server.
- Manual smoke test against a real guest confirms `exec`, interactive `shell`, and mount-time file writes still work.
