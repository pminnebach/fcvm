# Release notes

## v1.2.1 (2026-09-14)

Patch release: fixes a guest-networking race that could make `shell`/`exec` fail right after `start`. Earlier versions: [version history](https://github.com/pminnebach/fcvm/blob/main/docs/version_history.md).

### Fixes

- `shell` and `exec` now wait for guest SSH readiness (`guest.WaitSSH`) before connecting, instead of making a single unretried attempt. Previously, a guest whose networking was still settling right after `start` could make `fcvm shell` fail immediately with `ssh: connect to host ... port 22: No route to host`, even though `start` itself had already confirmed SSH was reachable moments earlier.
