# Release notes

## v1.2.0 (2026-07-31)

Review-backlog release: correctness and security hardening, CLI/guest polish, experimental vsock, and a one-line installer. Earlier versions: [version history](https://github.com/pminnebach/fcvm/blob/main/docs/version_history.md).

### Correctness and data safety

- Allocate the lowest free VM index (with a state-dir flock) instead of counting live VMs; refuse to steal an existing TAP device
- Validate VM ids before any root `RemoveAll` / jailer-tree path
- Host mounts no longer silently fall back to a copied block image — NFS failure is an error; use `method=block` explicitly
- Writable block mounts sync back to the host on `stop`
- `stop` matches PID plus `/proc` start time before signalling, then escalates to SIGKILL; `list` shows a real `STATUS` column

### Security

- NFS exports scoped to the guest address (not `*`); staging under the state dir instead of `/tmp`
- TAP/NAT rules live in a dedicated `FCVM` iptables chain; host `FORWARD` policy is untouched; `ip_forward` restored when the last VM stops
- Downloads verify SHA-256 (fail-closed for Firecracker releases), refuse plain HTTP without `--insecure`, and use connect/stall timeouts

### CLI and guest

- Context + signal handling, `SilenceUsage`, strict mount option parsing, shell-quoted `exec` arguments
- Guest DNS from host resolvers (configurable); mounts and env written as host files; env values single-quote escaped
- Auto-rebase default TAP `/30` when it collides with a host subnet; hard-error for an explicit colliding base

### Experimental

- Virtio-vsock opt-in via `--enable-vsock`, guest agent injection, and `fcvm vsock-exec`
- `fcvm download guest-agent` fetches the matching GitHub release (checksum verified); optional `--url`
- CNI and vsock require confirmation unless `--enable-experimental` is set (`fcvm experimental` lists them)

### Install and ops

- `install.sh`: curl|bash one-liner installs the latest release binary and Firecracker assets under sudo
- CI: skip root-only `chown` in `InjectSSHKey` when not running as root so package tests pass unprivileged

### Docs

- Host-side debugging guide ([debug.md](https://github.com/pminnebach/fcvm/blob/main/docs/debug.md)); jailer PID tracking notes; docs/example config updated for the above
