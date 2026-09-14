# Code review: usability and bugs

Scope: read-through of `README.md`, `docs/`, and the Go/shell source, cross-checked
against actual behavior (including a few live experiments) rather than docs alone.
No code was changed; findings below are ordered roughly by impact. File:line
references point at the current `main`.

## Findings

### 1. Every `exec`/`shell` call (and `start` with mounts) prints a spurious SSH warning

`guest/ssh.go:64` (`sshOpts`) sets `StrictHostKeyChecking=no` and
`UserKnownHostsFile=/dev/null` but does not add `-o LogLevel=ERROR` (or `=QUIET`).
OpenSSH prints `Warning: Permanently added '<ip>' (ED25519) to the list of known
hosts.` to stderr on *every* connection when host-key checking is disabled this
way — verified locally:

```
$ ssh -F /dev/null -i key -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null ... root@127.0.0.1 true
Warning: Permanently added '127.0.0.1' (ED25519) to the list of known hosts.
```

`guest.Exec` (`guest/ssh.go:108-115`), `guest.Shell` (`:127-133`), and
`guest.WriteFile` (`:118-125`) all set `cmd.Stderr = os.Stderr`, so this line lands
on every `fcvm exec`, every `fcvm shell`, and once per `fcvm start` that pushes a
mount table. Because fcvm always talks to a guest IP whose host key is brand new
(a new keypair-less handshake every time a VM is recreated — `UserKnownHostsFile`
is `/dev/null`, so nothing is ever actually remembered), this "warning" is not a
one-off notice, it is permanent noise on stderr for the CLI's two most-used
commands. For a tool meant to be driven by scripts/agents that watch stderr for
real problems, this is a real papercut.

`WaitSSH` (`:76-90`) is unaffected — it doesn't attach `Stderr`, so Go's `exec`
routes it to `/dev/null` — the noise is specific to `Exec`/`Shell`/`WriteFile`.

**Fix:** add `-o LogLevel=ERROR` (or `-o LogLevel=QUIET`) to `sshOpts`.

### 2. Shell completion breaks on an unrelated config error

`cmd/root.go:89-93`:

```go
rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
    switch cmd.Name() {
    case "experimental", "version", "help", "completion":
        return nil
    }
    c, err := loadConfig()
    ...
```

Cobra's auto-generated completion command is a *group* named `completion`; the
command that actually runs is one of its children — `bash`, `zsh`, `fish`,
`powershell` (and, for live tab-completion, the hidden `__complete` command).
`cmd.Name()` returns the leaf name, so none of these ever match `"completion"` in
the switch above, and the skip-list is effectively dead for real completion
invocations. They all fall through into `loadConfig()`. Reproduced:

```
$ cat /tmp/badcfg.yaml
vcpu-count: "not-a-number"
$ ./fcvm --config /tmp/badcfg.yaml completion bash
Error: decoding failed due to the following error(s):
'vcpu-count' cannot parse value as 'int64': strconv.ParseInt: invalid syntax
```

A single bad value in `~/.fcvm.yaml` now also breaks shell tab-completion (and,
worse, `__complete`, which fires on every `<TAB>` press), which has nothing to do
with config. **Fix:** check `cmd.Parent() != nil && cmd.Parent().Name() ==
"completion"` (or `cmd.Root().Find`-based check), or simply skip
`PersistentPreRunE` work when `cmd.Name() == cobra.ShellCompRequestCmd` /
the command tree root is `completion`.

### 3. CNI mode: NFS export can be scoped to the guest's own address

`vm/manager.go:379-396` (`setupMounts`):

```go
server := gateway
if server == "" {
    server = guestIP
}
```

`gateway` here is the CNI-reported gateway (`vm/manager.go:252-259`,
`resolveCNIAddrs` at `:410-428`). `resolveCNIAddrs` only errors when the CNI
result has no IP at all; it happily returns an empty `gateway` when
`ipCfg.Gateway == nil` (a legitimate case the code itself guards against with
`if ipCfg.Gateway != nil`). When that happens, the NFS `source` written into the
guest's mount table (`rootfs.MountRecord.Source = server + ":" + exp.ExportPath`)
becomes `<guestIP>:<path>` — the guest is told to NFS-mount an export from
*itself*, which can never work. This silently turns a mount request into a
guaranteed-broken mount instead of a clear "no gateway resolved" error. Static
TAP mode is unaffected (`gateway` is always `tapIP` there).

**Fix:** treat an empty CNI gateway as a hard error in `setupMounts` (or in
`resolveCNIAddrs`) rather than falling back to `guestIP`.

### 4. `Dockerfile.Default` / `Dockerfile.Kasm` depend on an undocumented base image

Both `Dockerfile.Default:4` and `Dockerfile.Kasm:4` start with:

```dockerfile
FROM fcvm-ubuntu-2604
```

`fcvm-ubuntu-2604` is not a real, pullable image — it's only produced by
separately building `Dockerfile.Ubuntu-2604` and tagging it yourself (e.g.
`docker build -f Dockerfile.Ubuntu-2604 -t fcvm-ubuntu-2604 .`). Nothing in
`docs/rootfs.md` or the README explains this two-step chain; it just lists the
four Dockerfiles side by side as if any of them can be built directly with
`fcvm build-rootfs --dockerfile <name>`. A user who reasonably tries
`fcvm build-rootfs --dockerfile ./Dockerfile.Default` gets an opaque Docker
failure (`pull access denied for fcvm-ubuntu-2604, repository does not exist`)
with no hint that a prerequisite build step is missing.

**Fix:** either document the build order in `docs/rootfs.md`, or make the
dependency explicit (e.g. commented-out `ARG BASE_IMAGE` lines are already
present but unused — wire them up, or add a one-line comment in each file
pointing at `Dockerfile.Ubuntu-2604`).

### 5. Dead `--` handling in `exec`/`vsock-exec` masks how args actually work

`cmd/exec.go:23-25` and `cmd/vsock_exec.go:26-28`:

```go
cmdArgs := args[1:]
if cmdArgs[0] == "--" {
    cmdArgs = cmdArgs[1:]
}
```

cobra/pflag already consumes the `--` separator before `RunE` ever sees `args`
(verified directly against this repo's pinned cobra/pflag versions — a `--` in
`SetArgs` never reaches `Run`). So `cmdArgs[0] == "--"` can never be true; this
branch is unreachable. It's harmless today, but it actively misleads whoever
reads it into thinking `--` needs manual handling, and it means the real
constraint — that `fcvm exec <id> -- <cmd>` requires the literal `--` whenever
`<cmd>` starts with something that looks like a flag (`-a`, `--foo`, etc.), or
pflag will try to parse it as an fcvm flag — isn't documented anywhere near this
code.

**Fix:** drop the dead branch, or replace it with a comment explaining that `--`
is mandatory-looking but actually consumed upstream by pflag.

### 6. Concurrent `vsock-exec` against the same VM can steal each other's output socket

`guest/vsock.go:42-58` (`ListenOutput`):

```go
func ListenOutput(udsPath string) (net.Listener, error) {
    path := vsock.OutputUDSPath(udsPath)
    _ = os.Remove(path)
    ln, err := net.Listen("unix", path)
    ...
```

Every `fcvm vsock-exec` invocation unconditionally unlinks and recreates the same
per-VM path (`<chroot>/vsock.sock_5253`). If two `vsock-exec` calls against the
same VM overlap (two terminals, or a script that retries after a slow first
call), the second call's `os.Remove` + `net.Listen` yanks the path out from under
the first listener: any new guest-initiated connection to that path is now
routed to the second listener, not the first, so the first invocation hangs
waiting on `ln.Accept()` forever (or until its own context/timeout gives up) and
never sees its output. This is scoped to the experimental vsock path, but it's a
real footgun with no guard or lock.

**Fix:** take a per-VM lock (or a PID-suffixed output path) around
`ListenOutput`/`VsockExec`, or document that only one `vsock-exec` at a time is
supported per VM.

### 7. Leftover debug/telemetry instrumentation in `files/install-kasm.sh`

`files/install-kasm.sh:36-46` and its call sites (`:167`, `:192`, `:207`, `:222`,
`:281`, `:299`, `:305`):

```bash
# #region agent log
_debug_log() {
  local hypothesis_id="$1" message="$2" data="$3"
  ...
  _debug_path="/tmp/debug-ab6709.log"
  { printf '{"sessionId":"ab6709",...,"hypothesisId":"%s","runId":"%s"}\n' ... }
    >> "${_debug_path}" 2>/dev/null || true
}
# #endregion
```

This reads like leftover instrumentation from an AI-assisted debugging session
(hardcoded `sessionId`/`hypothesisId`/`runId`, `#region agent log` markers) that
never got cleaned up before merge. It's not gated behind a flag — every real run
of `install-kasm.sh` (which a user's rootfs build can invoke inside the guest,
per `docs/rootfs.md`) silently appends JSON debug lines to
`/tmp/debug-ab6709.log` on the machine it runs on. Harmless functionally, but it
doesn't belong in a shipped helper script and will confuse anyone who finds the
file and doesn't know what session `ab6709` is.

**Fix:** remove the `_debug_log` function and its call sites.

### 8. Long-running steps give no progress feedback

`rootfs/docker.go:13` (`docker build ...).CombinedOutput()`), and similarly
`rootfs/ext4.go:60-64` (`truncate`/`mkfs.ext4`), buffer all output and print
nothing until the command finishes or fails. `fcvm build-rootfs` can legitimately
take minutes (Docker image build + export + `mkfs.ext4` over a multi-GB tree),
and during that whole window the CLI prints nothing at all — indistinguishable
from a hang. Same pattern for `fcvm download kernel`/`rootfs` on a slow link:
`assets/download.go`'s `DownloadFile` streams to disk but never reports bytes
transferred.

**Fix:** stream `docker build`'s output live (`cmd.Stdout = os.Stdout`) instead
of buffering, and/or print a "building..." heartbeat; a simple byte-count
progress line for downloads would help too.

## Minor / documentation notes

- `docs/cli.md`'s `--mount` flag help text (`cmd/start.go:52`) omits `auto` from
  the listed methods (`host:guest[:ro|rw|method=nfs|block|size=N]`) even though
  `method=auto` is valid and is in fact the default — easy to miss when skimming
  `--help` output.
- `cli.md` tells users to run `fcvm cleanup` to reclaim a crashed VM, but
  `fcvm stop <id>` on an already-dead VM actually does the same full teardown
  (`vm/manager.go:435-451` doesn't skip teardown when `IsRunning()` is false) —
  not wrong, just a bit confusing that the doc singles out `cleanup` as *the*
  recovery command when `stop` also works.
- `SetupNFSExport` (`network/nfs.go:33`) resolves a relative `--mount host:...`
  path with `filepath.Abs`, i.e. relative to the current working directory of
  the `fcvm start` invocation (not the state dir, not the user's home). This
  isn't documented in `docs/network.md` or `docs/cli.md`, and could surprise
  someone who runs `sudo fcvm start ...` from a different directory than they
  expect (`sudo` itself doesn't change `cwd`, but scripts/cron jobs sometimes
  do).
