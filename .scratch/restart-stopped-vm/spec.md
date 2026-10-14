Status: ready-for-agent

# Restart a stopped VM

## Problem Statement

As a user of fcvm, when I stop a VM I lose it. `fcvm stop` tears down the VM's network, removes its jailer tree and deletes its whole directory, including the disk, so installed packages, files and the VM's identity are gone. I cannot come back to a VM the next day, and a VM that crashed can only be thrown away with `cleanup` or blocked by a leftover `state.json` that makes `start` refuse. There is also no way to tell whether a VM that is not running was stopped on purpose or died.

## Solution

Stopping a VM becomes non-destructive. `stop` halts the VM and releases its host resources but keeps its disk, its VM configuration and its reserved network slot, leaving a **Stopped VM**. `start <id>` boots a stopped (or crashed) VM again from its preserved disk using its saved configuration, `restart <id>` stops and starts a VM in one step, and a new `delete <id>` is the deliberate way to remove a VM for good. `list` reports whether each VM is `running`, `stopped` (the user stopped it), `crashed` (it died without being stopped) or `legacy` (created before this change). Restart is a cold boot, not a memory snapshot restore. See `CONTEXT.md` for the vocabulary and `docs/adr/0001-stop-preserves-the-vm.md` for the decision record.

## User Stories

1. As a user, I want `fcvm stop <id>` to keep the VM's disk, so that my installed packages and files survive.
2. As a user, I want to run `fcvm start <id>` on a stopped VM, so that I can carry on from where I left off.
3. As a user, I want a restarted VM to boot from its preserved disk, so that my data is still there.
4. As a user, I want a restarted VM to keep the same configuration (resources, mounts, env, network mode), so that it behaves as it did before.
5. As a user, I want `fcvm start <id>` with `--mount` or `--env` on an existing VM to fail with a clear message, so that I do not think I changed a VM's shape when I did not.
6. As a user, I want that error to tell me to delete and recreate the VM, so that I know how to get a different shape.
7. As a user, I want `fcvm start <new-id>` to still create a new VM, so that the existing workflow keeps working.
8. As a user, I want `fcvm start` with no id to still generate an id and create a VM, so that quick throwaway VMs stay easy.
9. As a user, I want `fcvm start <id>` on a running VM to fail clearly, so that I do not start it twice.
10. As a user, I want `fcvm restart <id>` on a running VM to stop it and start it again, so that I can bounce a VM in one command.
11. As a user, I want `fcvm restart <id>` on a stopped VM to just start it, so that I do not need to remember its state.
12. As a user, I want `fcvm restart <id>` on a crashed VM to start it, so that recovery is one command.
13. As a user, I want `fcvm restart <unknown-id>` to fail with a "no such VM" error, so that a typo does not silently create a VM with default settings.
14. As a user, I want `fcvm start <id>` on a crashed VM to clean up what the crash left behind and then boot, so that I do not have to run cleanup first.
15. As a user, I want a failed restart to leave the VM stopped with its disk intact, so that a transient error never destroys my data.
16. As a user, I want a failed creation to still remove everything it half-created, so that failed first starts do not litter the state directory.
17. As a user, I want `fcvm delete <id>` to remove a VM's disk, configuration and reserved slot, so that I can reclaim disk space deliberately.
18. As a user, I want `fcvm delete <id>` to refuse a running VM unless I pass `--force`, so that I do not delete something in use by accident.
19. As a user, I want `fcvm delete <id> --force` to stop and then delete a running VM, so that I can remove it in one step.
20. As a user, I want `fcvm delete <unknown-id>` to fail clearly, so that I know nothing was removed.
21. As a user, I want `fcvm cleanup` to keep reclaiming orphaned resources, so that broken leftovers can still be removed.
22. As a user, I want `cleanup` and `delete` to remove the same things, so that neither leaves debris the other would remove.
23. As a user, I want `fcvm list` to show `running`, `stopped`, `crashed` and `legacy` statuses, so that I can tell at a glance what happened to each VM.
24. As a user, I want a VM I stopped to show as `stopped`, so that I know it is intentionally down.
25. As a user, I want a VM whose process died without a stop to show as `crashed`, so that I notice failures.
26. As a user, I want `list` to decide this by comparing the recorded stop with the real process, so that the status reflects reality and not only a flag.
27. As a user, I want `list` to show when a VM was stopped, so that I know how long it has been down.
28. As a user, I want `list` to show a stopped VM's guest IP in TAP mode, so that I know the address it will come back on.
29. As a user, I want `list` to show `-` for a stopped VM's guest IP in CNI mode, so that I am not told an address that may change.
30. As a user, I want a stopped VM's guest IP to be stable in TAP mode, so that scripts and known_hosts entries stay valid.
31. As a user, I want a stopped VM to keep its reserved network slot, so that a new VM cannot take its IP while it is away.
32. As a user, I want a stopped VM to hold no TAP device, firewall rules or NFS exports, so that it costs the host nothing while stopped.
33. As a user, I want the host's `FCVM` firewall chain and `ip_forward` setting to be released when the last running VM stops, so that stopped VMs do not hold host networking open.
34. As a user, I want NFS exports to be recreated on every start against the VM's current guest IP, so that mounts work in CNI mode after the IP changes.
35. As a user, I want a writable block mount's image to survive a restart without being rebuilt from the host directory, so that guest-side changes are not overwritten with older host content.
36. As a user, I want `stop` to still sync writable block mounts back to the host directory, so that host files stay current.
37. As a user, I want env baked into the rootfs to still be present after a restart, so that processes see the same environment.
38. As a user, I want `fcvm exec` and `fcvm shell` on a stopped VM to say the VM is stopped and how to start it, so that the error is actionable.
39. As a user, I want `fcvm stop` on an already stopped VM to succeed without error, so that scripts can call it repeatedly.
40. As a user, I want `fcvm stop` on a crashed VM to clean up its leftovers and mark it stopped, so that I can acknowledge a crash.
41. As a user, I want `fcvm stop <unknown-id>` to fail clearly, so that I notice typos.
42. As a user, I want an interrupted `stop` or a host reboot to show the VM as `crashed`, so that the status never claims an intentional stop that did not finish.
43. As a user with VMs created before this change, I want them shown as `legacy`, so that I understand why they cannot be restarted.
44. As a user with a legacy VM, I want `stop` and `cleanup` to still remove it, so that I can get rid of it.
45. As a user, I want the release notes to say that `stop` no longer deletes, so that I know to use `delete` to reclaim disk space.
46. As a maintainer, I want one shared teardown path for stop, delete, cleanup and failed-start rollback, so that new resources only need cleanup written once.
47. As a maintainer, I want the stop record, VM configuration and runtime state held in separate files, so that "running", "stopped" and "crashed" are unambiguous from what is on disk.
48. As a maintainer, I want the VM configuration written once and never modified, so that a restart can never drift from how the VM was provisioned.
49. As a maintainer, I want VM ids still validated before being used as path components, so that the new commands cannot be used to delete outside the state directory.
50. As a maintainer, I want the existing guards on `RemoveAll` calls kept on every deletion path, so that the root-run delete cannot escape its tree.

## Implementation Decisions

- **Vocabulary:** use the terms in `CONTEXT.md` (VM, Stopped VM, Crashed VM, VM configuration, Deleted VM, Start, Restart). "Sandbox" is an alias, not a term to use.
- **Per-VM files:** the VM directory holds a configuration record (written once at creation, immutable), a runtime state record (present only while the VM runs) and a stop record containing the stop time (written by a deliberate stop, removed when the VM next starts).
- **Configuration record:** must capture the full effective shape needed to boot again: machine resources, kernel args and other machine knobs, network mode, network index, mounts, env, whether vsock is enabled and the vsock CID, and the jailer uid/gid. Today only env and mounts are persisted, so the persisted shape must grow.
- **Status derivation:** `running` when the recorded process is alive and identified by its PID plus start time; `stopped` when there is a stop record and no live process; `crashed` when there is a runtime state record, no stop record and no live process; `legacy` when a VM has a runtime state record but no configuration record. Status is computed from files plus the live process, never stored.
- **Stop:** signals the process as today (PID-safe, SIGTERM then SIGKILL after the stop timeout), syncs writable block mounts back to the host directory, releases TAP/CNI, firewall rules and NFS exports, removes the jailer tree, writes the stop record, and removes the runtime state record last. It keeps the VM directory, rootfs copy, block mount images and configuration record. It is idempotent on an already stopped VM. On a crashed VM it performs the leftover teardown and records the stop. Releasing the host `FCVM` chain and `ip_forward` happens when no VM remains running, not when none remain on disk.
- **Start of an existing VM:** loads the configuration record and reuses its network index and guest identity; if leftovers from a crash exist, tears them down first; skips copying the template rootfs and skips rebuilding block mount images; re-injects nothing that is already baked in unless the lifecycle needs it; recreates TAP/CNI, firewall rules, jailer tree and NFS exports; boots; writes the runtime state; removes the stop record. `--mount` and `--env` supplied for an existing VM are an error.
- **Start of a new VM:** unchanged behaviour, except it also writes the configuration record. If creation fails, everything created is removed as today.
- **Failed restart rollback:** releases only runtime resources acquired during that attempt and leaves the VM stopped with the stop record restored. The disk and configuration are never removed on this path.
- **Restart command:** `restart <id>` stops the VM if running, then starts it. An unknown id is a "no such VM" error. It never creates a VM.
- **Delete command:** removes everything for a VM (disk, block images, configuration, jailer tree, leftovers) and frees its network slot. Refuses a running VM unless forced; with `--force` it stops then deletes. Unknown id is an error.
- **Cleanup command:** keeps its best-effort orphan reclaim role (works with or without records, `--all` walks the state directory) and shares the teardown routine with `delete`.
- **Network index allocation:** the set of claimed indices must include stopped VMs (via their configuration records), not only running ones, so a stopped VM's slot and guest IP cannot be taken. The lock that guards allocation is unchanged.
- **TAP versus CNI:** the guest IP is stable across restarts only in TAP mode. In CNI mode the address, gateway and MAC are re-resolved at each start and NFS exports are scoped to the new address. `list` shows the guest IP of a non-running VM only in TAP mode and `-` otherwise.
- **List output:** gains the new statuses and a column for the stop time. PID and uptime show `-` when not running.
- **Exec and shell:** return an actionable error naming the VM's status when it is not running.
- **Legacy VMs:** no migration. They are reported as `legacy`, can be stopped (which deletes them as before) or cleaned up, and cannot be started again.
- **Block mounts:** the image is authoritative while the VM is stopped. Stop syncs it back to the host directory, and start of an existing VM reuses the image.
- **Safety:** ids stay validated before use as path components, and each deletion keeps the existing mount and `RemoveAll` guards. The directory layout, `state.json` description and stop/cleanup sections in the architecture docs, the CLI reference and the release notes must be updated.

## Testing Decisions

- A good test exercises external behaviour: what is on disk, what `list` reports, and what the command returns. It does not assert on which internal helper ran.
- **Manager seam (primary):** tests drive `vm.Manager` against a temp state directory with host commands recorded rather than executed (as the existing cleanup tests do) and the Firecracker launch stubbed. They cover: stop keeps disk, configuration and index and writes a stop record; start of a stopped VM reuses the disk and index and clears the stop record; start of a crashed VM tears down leftovers first; failed restart leaves the disk and configuration intact and the VM stopped; failed creation removes everything; restart of running, stopped, crashed and unknown VMs; delete of stopped, running (refused and forced) and unknown VMs; status classification through `List` for running, stopped, crashed and legacy, including a recycled PID; allocation never reuses a stopped VM's index; block mount image reuse; and the host `FCVM` chain release when the last running VM stops.
- **cmd seam:** cobra-level tests cover rejecting `--mount` and `--env` for an existing VM, `restart` of an unknown id, `delete` with and without `--force`, and `list` output formatting for each status including the TAP versus CNI guest IP column.
- **Prior art:** the temp-state-directory and recorded-command style in the existing cleanup, liveness, state, index and mount write-back tests, and the cobra tests in the existing command test files.
- **Manual smoke test on real hardware:** start a VM, write a file in the guest, `stop`, `start`, confirm the file survives and the IP is unchanged in TAP mode; kill the Firecracker process and confirm `list` shows `crashed` and `start` recovers it. This needs root and `/dev/kvm`.

## Out of Scope

- Firecracker snapshots and resuming memory state.
- Changing a stopped VM's configuration (resources, mounts, env) without deleting it.
- Migrating VMs created before this change.
- Graceful Firecracker API shutdown before signalling on stop (tracked separately in its own plan).
- `fsck` of a dirty rootfs after a crash.
- Sparse copying of the rootfs to reduce disk use of stopped VMs.
- Automatic expiry or garbage collection of stopped VMs.

## Further Notes

- Stopped VMs keep a full rootfs copy each, so disk use grows until `delete`; the known non-sparse copy makes this worse and is a candidate follow-up.
- A stopped VM reserves its network index indefinitely, so the number of VMs that can exist is bounded by the index range even if few are running.
- A crashed VM's rootfs may be dirty; the guest's own boot is relied on to recover the filesystem.
- The decision is recorded in `docs/adr/0001-stop-preserves-the-vm.md`.
