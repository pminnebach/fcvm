# Destructive filesystem guards

Two places where fcvm runs `os.RemoveAll` as root on a path it should not trust: unvalidated VM ids, and temp dirs whose unmount may have failed.

## Issue 1 — VM id is an unvalidated path component

### Evidence

Confirmed with a throwaway test against the real helper:

```go
RemoveState(stateDir, "../../victim")   // returns nil; deletes <base>/victim
```

`filepath.Join` cleans `..` away, so the id escapes the state directory. Reachable as root from `fcvm cleanup <id>`, `fcvm stop <id>`, and `fcvm start <id>`.

### Root cause

Every state path is built by joining the id verbatim ([vm/state.go](../../vm/state.go)):

```go
func statePath(stateDir, id string) string {
	return filepath.Join(stateDir, "vms", id, "state.json")
}

func RemoveState(stateDir, id string) error {
	return os.RemoveAll(filepath.Join(stateDir, "vms", id))
}
```

The same id also reaches `Manager.jailerTreeDir` → `removeJailerTree` → `os.RemoveAll`, the jailer `ID` passed to Firecracker, the NFS export directory name, and the CNI netns path. No command validates it; `cleanup` does not even declare an `Args` validator.

### Fix

One validator, called from the shared path helpers rather than from each command — patching only `cleanup` would leave `stop` and `start` reachable.

```go
// ValidateID rejects ids that are not a single safe path component.
func ValidateID(id string) error
```

Rules: non-empty, not `.` or `..`, no `/` or `\`, no NUL, and `filepath.Base(id) == id`. Restricting to `[A-Za-z0-9_.-]` is tighter and costs nothing — the ids fcvm generates itself (`vm-<unix>`) already satisfy it, and the id ends up in an interface name, an export filename, and a chroot path, all of which prefer a conservative alphabet.

Call sites: `statePath`, `SaveState`, `RemoveState`, and `Manager.jailerTreeDir`. `LoadState` should reject before touching the disk so a bad id produces a clear error rather than a confusing `ErrNotExist`. Add `cobra.MaximumNArgs(1)` to `cleanup` while in the file — it currently accepts and silently ignores extra arguments.

Length matters too: `TapDevName` formats `fcvm-tap-%d` from the index so it is safe, but the export filename and chroot path are id-derived. Cap the id at a length that keeps `/etc/exports.d/fcvm-<id>-<n>.exports` sane — 64 characters is generous.

## Issue 2 — `RemoveAll` over a possibly-still-mounted temp dir

### Root cause

This is the same bug already fixed in [network/nfs.go](../../network/nfs.go), where `removeExportDirWith` refuses to delete while `isMountPoint` reports the share is mounted. The sibling call sites were never updated.

[assets/patch.go](../../assets/patch.go), both `PatchExt4` and `PatchNetwork`:

```go
dir, err := os.MkdirTemp("", "fcvm-mount-*")
defer os.RemoveAll(dir)
...
defer exec.Command("umount", mountPoint).Run()   // error dropped
```

and [rootfs/hooks.go](../../rootfs/hooks.go) `PatchMounted`, which holds the inner `mount`/`umount` pair. If the unmount fails — a stray process in the tree, a lazy-unmount race, an EBUSY — the outer `RemoveAll` recurses *into the mounted image* and deletes the rootfs contents. Today the mount source is an fcvm-owned ext4 copy rather than user data, which is the only reason this has not caused a visible loss yet.

### Fix

Reuse what already exists instead of writing a second guard. Export the mount-point check from `network` (or move it to a small shared helper package if the import direction is awkward — `network` importing nothing from `assets`/`rootfs` means `assets` → `network` is fine today), then:

1. Make `PatchMounted` return the unmount error instead of dropping it, so callers know the state.
2. In `PatchExt4` / `PatchNetwork`, replace the bare `defer os.RemoveAll(dir)` with a deferred helper that skips removal and logs loudly when the mount point is still mounted.
3. Apply the same guard to the block-image write-back added in [mount-writeback.md](mount-writeback.md), where the mount source *is* user data.

## Locked decisions

| Topic | Choice |
|-------|--------|
| Validation point | Shared state/jailer path helpers, not individual commands |
| Alphabet | `[A-Za-z0-9_.-]`, max 64 chars, not `.` or `..` |
| Error shape | Wrapped, names the offending id, distinct from `ErrNotExist` |
| Mount guard | Reuse `isMountPoint` from `network`; do not write a second implementation |
| Failed unmount | Skip removal, return/log an error; never force |

## Code touch list

| Area | Change |
|------|--------|
| [vm/state.go](../../vm/state.go) | `ValidateID`; call from `statePath`, `SaveState`, `RemoveState`, `LoadState` |
| [vm/manager.go](../../vm/manager.go) | Validate in `jailerTreeDir`; surface validation errors from `Start`/`Stop`/`Cleanup` |
| [cmd/cleanup.go](../../cmd/cleanup.go) | `Args: cobra.MaximumNArgs(1)` |
| [network/nfs.go](../../network/nfs.go) | Export the mount-point check for reuse |
| [assets/patch.go](../../assets/patch.go) | Guarded cleanup in `PatchExt4` and `PatchNetwork` |
| [rootfs/hooks.go](../../rootfs/hooks.go) | `PatchMounted` returns the unmount error |

## Check to leave behind

The traversal test is three lines of setup and is the one that matters:

```go
// RemoveState must refuse an id that escapes the state dir.
err := RemoveState(stateDir, "../../victim")
// assert err != nil and that victim/ still exists
```

The mount-guard case is already covered in spirit by `TestRemoveExportDirSkipsWhileMounted` in [network/nfs_test.go](../../network/nfs_test.go); mirror that structure for the patch helpers if the shared guard is refactored, and reuse its root-skip.

## Non-goals

- No sandboxing or chroot of fcvm itself.
- Do not rename existing VM directories or migrate state; validation applies going forward and old-but-valid ids keep working.
- Do not add generic path-safety abstractions beyond the one validator.

## Success criteria

- `sudo fcvm cleanup ../../anything` errors out and deletes nothing.
- `sudo fcvm start 'foo/bar'` is rejected before any directory is created.
- A patch helper whose unmount fails leaves the mount in place and reports it, instead of deleting through it.
