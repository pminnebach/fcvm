# NFS export hardening

Host directories shared into a guest are currently exported to every host that can reach the box, and staged under a world-writable `/tmp`.

## Issue 1 — Exports are world-scoped

### Symptom

```bash
sudo fcvm start dev --mount /home/me/src:/src
showmount -e <host-ip>     # from any other machine on the network
# /tmp/fcvm-exports/dev-0/share  *
```

Any host that can reach port 2049 can mount and write `/home/me/src`, squashed to the directory owner's uid.

### Root cause

The Linux branch of `nfsExportLine` ([network/nfs.go](../network/nfs.go)) hardcodes the `*` client pattern:

```go
return fmt.Sprintf("%s *(%s,sync,no_subtree_check,all_squash,anonuid=%d,anongid=%d)", target, rw, uid, gid)
```

The darwin branch above it already scopes to `-network 172.16.0.0 -mask 255.255.0.0`, so the intent existed and was only implemented on one platform. `TestNFSExportLineMapsToOwner` asserts the squash options and the absence of `no_root_squash`, but nothing asserts the client scope — which is why this went unnoticed.

### Fix

Pass the allowed client into `nfsExportLine` instead of assuming one. The caller knows it:

- **TAP mode** — the guest's `/30`. `SubnetForIndex` already produced `tapIP`/`guestIP`; export to the guest IP alone (`172.16.N.2(rw,…)`), which is the tightest correct scope.
- **CNI mode** — the guest IP is not known until after `machine.Start`, and `Start` already defers the mount metadata for exactly this reason. Move `SetupNFSExport` on the CNI path to after `resolveCNIAddrs` so the resolved IP can scope the export, or scope to the CNI subnet from the resolved gateway if per-IP proves impractical with the plugin's addressing.

`SetupNFSExport(hostPath, vmID string, readOnly bool)` gains a client parameter; it has one caller. Never emit `*`: if the client is empty, that is a programming error and should fail the start rather than fall back to world.

## Issue 2 — Export staging lives in `/tmp`

### Root cause

```go
exportDir := filepath.Join("/tmp", "fcvm-exports", vmID)
```

Root then does `MkdirAll`, `mount --bind`, `WriteFile`, and `RemoveAll` beneath a world-writable directory. An unprivileged local user can pre-create `/tmp/fcvm-exports` (or a per-id path, since ids are predictable — `vm-<unix-timestamp>`) as a symlink or as a directory they own, and influence where root binds and removes. The existing `isMountPoint` guard prevents the worst outcome but does not address the staging location itself.

### Fix

Move staging under the state directory, which is already root-owned and already the home for per-VM artifacts: `<state-dir>/exports/<vmID>/share`. This also makes teardown consistent with everything else fcvm cleans up, and removes the last hardcoded `/tmp` path in the export code.

`network` currently takes no config, so pass the export root in from `vm.Manager` rather than importing `config` into `network`. `TeardownNFSExport` and `TeardownNFSExportsForVM` need the same parameter — note `cleanupVM` calls the latter for orphans without state, so the root must be derivable from config alone (it is: `cfg.StateDir`).

While in the file: `TeardownNFSExportsForVM` calls `TeardownNFSExport(vmID)` once inside the prefix loop and once unconditionally at the end. Harmless, but the second call is redundant when the loop already matched.

## Locked decisions

| Topic | Choice |
|-------|--------|
| TAP client scope | The guest IP, single host |
| CNI client scope | Resolved guest IP after `Start`; export setup deferred to match |
| Empty client | Hard error, never `*` |
| Staging root | `<state-dir>/exports/<vmID>`, passed in from the manager |
| Squash options | Unchanged (`all_squash` + `anonuid`/`anongid` from the host path owner) |
| Existing exports | No migration; `cleanup --all` removes stale `/etc/exports.d/fcvm-*` as today |

## Code touch list

| Area | Change |
|------|--------|
| [network/nfs.go](../network/nfs.go) | Client parameter through `nfsExportLine` / `SetupNFSExport`; export root parameter; drop redundant teardown call |
| [vm/manager.go](../vm/manager.go) | Pass guest IP + export root; defer CNI export setup until after address resolution |
| [docs/network.md](../docs/network.md) | State the export scope and the new staging path |

## Check to leave behind

Extend `TestNFSExportLineMapsToOwner` ([network/nfs_test.go](../network/nfs_test.go)) with the assertion that was missing: the line contains the guest IP as the client and does **not** contain `*`. It is two lines in a test that already exists and needs no root.

## Non-goals

- Do not switch to NFSv4-only, Kerberos, or a userspace NFS server.
- Do not manage `/etc/exports` itself; the `exports.d` drop-in approach stays.
- Do not add firewall rules for 2049 here — host-level packet policy is [host-network-scope.md](host-network-scope.md).

## Success criteria

- `showmount -e` from another host lists the export with the guest IP as the only allowed client.
- A mount attempt from a non-guest address is refused.
- No fcvm-created path under `/tmp` remains in the NFS code path.
