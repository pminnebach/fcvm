# Mount fallback data loss

`--mount` can silently throw away everything the guest wrote. Make the block fallback opt-in, and sync it back on stop when it is used.

Supersedes the older TODO line "Sync block-fallback mount images back to the host directory on stop".

## Symptom

```bash
sudo fcvm start work --mount /home/me/project:/project   # NFS unavailable → warning on stderr
sudo fcvm exec work -- sh -c 'echo result > /project/out.txt'
sudo fcvm stop work                                       # exit 0, /home/me/project unchanged
```

The user asked for a mount, got a one-way copy, and lost the writes with a zero exit code.

## Root cause

In `Manager.Start` ([vm/manager.go](../vm/manager.go)) the NFS path degrades to a block device on *any* error — `exportfs` missing, no NFS server, bind mount refused — and the only signal is a log line:

```go
exp, err := network.SetupNFSExport(mount.Host, exportID, mount.Mode == "ro")
if err != nil {
	m.log.Warnf("NFS unavailable for %s: %v; falling back to block device", mount.Host, err)
	method = "block"
}
```

`syncDirToExt4` then copies the host directory into `vms/<id>/mount-N.ext4`, which is attached as a virtio drive. Nothing reads that image back: `teardownState` removes the whole VM state dir, image included. The block path is a copy pretending to be a mount.

Two smaller defects live in the same function:

- `syncDirToExt4` hardcodes `truncate -s 512M`, so any host directory over ~500 MB fails at `mkfs.ext4` with an opaque error, even though `build-rootfs` already has a `--size` flag.
- The `mount.Mode == "ro"` intent is dropped entirely on the block path — the drive is attached with `IsReadOnly: false` ([vm/fc_config.go](../vm/fc_config.go)), so a read-only mount request becomes a writable scratch copy.

## Locked decisions

| Topic | Choice |
|-------|--------|
| Default method | `auto` means NFS only. If NFS setup fails, `Start` fails with the underlying error |
| Block usage | Opt-in per mount: `--mount host:guest:method=block` or `method: block` in config |
| Write-back | Block mounts sync image → host directory on `stop`, before the state dir is removed |
| Read-only block mounts | Attach with `IsReadOnly: true` and skip write-back |
| Image size | Sized from the source directory (`du -sb` + headroom), with a `size` mount option to override |
| Crash case | `fcvm cleanup <id>` performs the same write-back when a block image and its host path are still recorded in state |
| Conflict policy | Write-back mirrors the image over the host dir (`rsync -a --delete` semantics); no three-way merge |

## Fix

1. Extend the mount option grammar so `method` and `size` are settable per mount, and reject unknown keys. `mountFlag` in [cmd/root.go](../cmd/root.go) is the single parse point (see [cli-ergonomics.md](cli-ergonomics.md), which tightens the same function).
2. Delete the implicit fallback in `Start`: on `SetupNFSExport` error, wrap and return. Mention `method=block` in the error text so the workaround is discoverable.
3. Size the ext4 image from the source tree instead of the 512M constant; keep a floor so tiny directories still get a valid filesystem.
4. Honour `Mode == "ro"` when appending the drive in `buildFirecrackerConfig`.
5. Add write-back to the stop path. `State.Mounts` already records `Method`, `Host`, and `Device`, so the data needed is persisted. Mount the image on a temp dir, `rsync -a --delete` back to `MountState.Host`, unmount, then continue teardown. Guard the unmount/remove sequence with the `isMountPoint` check described in [destructive-path-guards.md](destructive-path-guards.md) — a failed unmount here would otherwise `RemoveAll` into the user's data.
6. Run write-back before `teardownState` deletes the VM directory, and log clearly when it happens.

## Code touch list

| Area | Change |
|------|--------|
| [cmd/root.go](../cmd/root.go) | `mountFlag` parses `method=` / `size=`; rejects unknown options |
| [config/config.go](../config/config.go) | `MountConfig.Size`; validate `Method` against `auto`/`nfs`/`block` |
| [vm/manager.go](../vm/manager.go) | Remove silent fallback; size images from source; write-back in `Stop`/`cleanupVM` |
| [vm/fc_config.go](../vm/fc_config.go) | `IsReadOnly` from mount mode |
| [docs/network.md](../docs/network.md) | Document that block mounts are copies with sync-on-stop, and their failure modes |
| [docs/cli.md](../docs/cli.md) | New mount option syntax |

## Check to leave behind

One test in `vm/` that does not need Firecracker: build a source directory with a known file, run the image-build helper, mount-free-verify by running the write-back helper against a *second* directory, and assert the file arrives. If loopback mounting is unavailable in CI, split the helper so the "copy image contents to host dir" step takes a source path and can be exercised directly — the regression being guarded is "stop discards writes", and that lives in the copy step, not in the loop mount.

Plus one table-driven assertion that `mountFlag` rejects an unknown `method=` value.

## Non-goals

- No live write-through (block mounts stay copies; NFS is the live path).
- No conflict resolution or merge strategy beyond mirror-on-stop.
- Do not make block the default, and do not remove it — it is the escape hatch for hosts without NFS.
- Do not implement virtio-fs here; that is a separate future option.

## Success criteria

- With no NFS server present, `fcvm start --mount …` fails with an actionable error instead of silently copying.
- With `method=block`, guest writes appear in the host directory after `fcvm stop`.
- A read-only block mount is not writable in the guest and is not synced back.
- Mounting a directory larger than 512 MB works.
