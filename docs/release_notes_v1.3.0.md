# Release notes

## v1.3.0 (Unreleased)

Stopped VMs can now be started again.

### Behaviour change: `stop` no longer deletes the VM

`fcvm stop <id>` used to remove the VM's whole directory, including its disk. It now stops the process and releases the TAP device, firewall rules, NFS exports and jailer tree, but **keeps the disk, the configuration and the network slot**. Disk space is no longer freed by `stop`: each stopped VM keeps its own copy of the rootfs until you run `fcvm delete <id>`.

### New

- `fcvm start <id>` boots a stopped or crashed VM again from its preserved disk with the configuration it was created with (a cold boot, not a memory restore). `--mount` and `--env` are rejected for an existing VM.
- `fcvm restart <id>` stops a running VM and starts it again, or just starts a stopped or crashed one. It never creates a VM.
- `fcvm delete <id>` removes a VM, its disk and its network slot. A running VM needs `--force`.
- `fcvm list` shows `running`, `stopped`, `crashed` or `legacy`, and when a VM was stopped. A VM you stopped is told apart from one whose process died.
- `fcvm exec`, `shell`, `attach` and `vsock-exec` on a VM that is not running now say so and how to start it.
- A stopped VM keeps its network index, so in TAP mode it comes back on the same guest IP and a new VM cannot take it. In CNI mode the address is assigned at each start.
- Per-VM records are now split: `config.json` (written once), `state.json` (while running) and `stopped.json`.

### Things to know

- `fcvm cleanup --all` removes the same things as `delete`, so it deletes stopped VMs and their disks too.
- VMs created before this release have no `config.json`. They show as `legacy` and cannot be started again; `stop` or `cleanup` removes them. There is no migration.
- `stop`, `delete` and `cleanup` now wait for the VM process to be gone before touching its disk, and fail without changing anything if it survives SIGKILL. They also take the state lock, so they no longer race a `start` in progress.
- A crashed VM's block mount images are copied back to the host only if `e2fsck -fn` passes. This adds `e2fsck` (e2fsprogs) as a host tool.
- A crashed VM's rootfs may be dirty; the guest's own boot recovers the filesystem and fcvm does not run `fsck`.
