# Release notes

## v1.3.0 (2026-10-14)

Restart release: a stopped VM keeps its disk and can be started again, with `restart` and `delete` commands and a `list` that tells a stopped VM from a crashed one. Also fixes VMs dying moments after `start`. Earlier versions: [version history](https://github.com/pminnebach/fcvm/blob/main/docs/version_history.md).

### Behaviour change

- `stop` no longer deletes the VM. It stops the process and releases the TAP device, firewall rules, NFS exports and jailer tree, but keeps the disk, the configuration and the network slot. Each stopped VM keeps its own rootfs copy until `fcvm delete <id>`, so `stop` no longer frees disk space
- `cleanup` removes the same things as `delete`, so `cleanup --all` now deletes stopped VMs and their disks too

### New

- `fcvm start <id>` boots a stopped or crashed VM again from its preserved disk with the configuration it was created with. This is a cold boot, not a memory restore. `--mount` and `--env` are rejected for an existing VM; delete it and start it again to change them
- `fcvm restart <id>` stops a running VM and starts it again, or starts a stopped or crashed one. It never creates a VM
- `fcvm delete <id>` removes a VM, its disk and its network slot. A running VM needs `--force`
- `fcvm list` reports `running`, `stopped`, `crashed`, `legacy` or `incomplete`, and a `STOPPED` column. A VM you stopped is told apart from one whose process died
- `exec`, `shell`, `attach` and `vsock-exec` on a VM that is not running say what state it is in and how to proceed
- A stopped VM keeps its network index, so in TAP mode it returns on the same guest IP and no new VM can take it. In CNI mode the address is assigned at each start
- Per-VM records are split into `config.json` (written once), `state.json` (while running) and `stopped.json`; each VM also gets a `console.log` ([architecture](https://github.com/pminnebach/fcvm/blob/main/docs/architecture.md), [ADR 0001](https://github.com/pminnebach/fcvm/blob/main/docs/adr/0001-stop-preserves-the-vm.md))

### Correctness and data safety

- `stop`, `delete` and `cleanup` wait for the VM process to be gone before touching its disk, and change nothing if it survives SIGKILL
- `stop`, `delete`, `cleanup` and the stop half of `restart` take the state lock, so they cannot release the shared host network under a `start` in progress
- A crashed VM's writable block mount images are copied back to the host only if `e2fsck -fn` passes, because the copy deletes host files missing from the image. `e2fsck` (e2fsprogs) is now a host tool
- A failed restart releases only what it took and leaves the VM stopped with its disk intact; a failed creation still removes everything
- A record that cannot be parsed is an error, never "no such VM": `start` no longer falls through to creation (whose cleanup deleted the disk), and index allocation refuses to run rather than free that VM's index
- A creation that never finished shows as `incomplete` and is never booted

### Fixes

- A VM could die moments after `start` returned. The Firecracker SDK stops the VM when the context it was started with is cancelled, and the command's context is cancelled as the command returns, so survival depended on a race with the process exiting. Firecracker now starts with a context that outlives the command. Ctrl+C while `start` waits for the guest still stops the VM: one being created is removed, one being restarted goes back to `stopped`
- Firecracker's console no longer goes to the terminal `start` ran from. It is written to `<state-dir>/vms/<id>/console.log`, appended across boots

### Upgrade notes

- VMs created before this release have no `config.json`. They show as `legacy` and cannot be started again; `stop` or `cleanup` removes them. There is no migration
- A crashed VM's rootfs may be dirty; the guest's own boot recovers the filesystem and fcvm does not run `fsck`

### Docs

- [cli.md](https://github.com/pminnebach/fcvm/blob/main/docs/cli.md), [architecture.md](https://github.com/pminnebach/fcvm/blob/main/docs/architecture.md), [network.md](https://github.com/pminnebach/fcvm/blob/main/docs/network.md) and [debug.md](https://github.com/pminnebach/fcvm/blob/main/docs/debug.md) cover the new lifecycle, statuses and records; `CONTEXT.md` defines the VM vocabulary
