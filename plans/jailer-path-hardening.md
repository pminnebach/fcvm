# Jailer path trustworthiness

Refuse to start when Firecracker/jailer binaries or the chroot base directory (and parents) are group- or world-writable. Fail closed; offer a explicit escape hatch for local development.

## Goal

Match Firecracker’s prod-host guidance: the host paths the jailer trusts must not be writable by untrusted users. fcvm should check before launching and error with a clear message.

## Symptom

An operator (or package install) can leave `firecracker`, `jailer`, or `~/.fcvm/jailer` mode `0777` / group-writable. A local user who can write those paths can replace the binary or plant content the jailer will execute or chroot into — while fcvm still happily starts VMs as root.

## Root cause

[`vm/manager.go`](../vm/manager.go) passes `cfg.FirecrackerBin`, `cfg.JailerBin`, and `cfg.Jailer.ChrootBaseDir` into SDK `JailerConfig` without checking ownership or permission bits. There is no preflight beyond “path exists / is executable” style checks (if any).

## Locked decisions

| Topic | Choice |
|-------|--------|
| What to check | `firecracker-bin`, `jailer-bin`, `chroot-base-dir`, and each parent up to `/` (or until a trusted root) |
| Rules | Prefer root-owned; reject group-writable or world-writable (`mode & 0022 != 0`). Symlink final targets must satisfy the same rules |
| Failure mode | Fail closed on `start` with an error naming the path and the bad mode/owner |
| Escape hatch | `--insecure-paths` (and/or config `jailer.insecure-paths: true`) for intentional dev setups; log a warning when used |
| When | Check on start (and optionally `doctor` later); not on every list/status |

## Fix

1. Add a small helper (e.g. `validateTrustedPath(path string) error`) that `Lstat`/`Stat`s the path and parents, rejects bad modes, and preferably requires uid 0 for binaries and chroot base in production.
2. Call it from the start path before jailer launch, using resolved absolute paths from config.
3. Wire `--insecure-paths` / config to skip the check (with stderr warning).
4. Document in configuration / prod-host docs: recommended `chown root:root` and `chmod go-w` on bins and jailer base.

## Code touch list

| Area | Change |
|------|--------|
| New helper (prefer [`vm/`](../vm/) or [`config/`](../config/) — keep it boring) | Path walk + mode/owner checks |
| [`vm/manager.go`](../vm/manager.go) or start command | Call before machine start |
| [`config/config.go`](../config/config.go), [`cmd/`](../cmd/) | Escape-hatch flag/field |
| Docs | Short “path permissions” note |

## Check to leave behind

Table-driven unit test over fake `fs.FileInfo` or temp dirs: world-writable fails; `0755` root-owned (or test-uid owned in unit tests with a test hook) passes; `--insecure-paths` skips. Prefer temp dirs with `chmod` so the test hits real `Stat` without mocking the world.

## Non-goals

- No mandatory SELinux/AppArmor policy.
- No rewriting package-manager paths.
- Do not change default uid/gid of the jailed process (see [jailer-isolation.md](jailer-isolation.md)).
- Do not recursively audit every file inside an existing chroot tree on each start (bins + chroot-base + parents only).

## Success criteria

- `fcvm start` fails on group/world-writable jailer/firecracker/chroot-base with a clear path in the error.
- `--insecure-paths` allows start and prints a warning.
- Hardened paths still start successfully.
