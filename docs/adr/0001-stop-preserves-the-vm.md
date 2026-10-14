# Stop preserves the VM; restart is a cold boot

`fcvm stop` used to delete the VM's directory, so nothing could be restarted. It now halts the process and releases runtime resources (TAP, firewall rules, NFS exports, jailer tree) but keeps the disk, the VM configuration and the reserved network index; deleting is a separate `delete` command. A restart is a cold boot from the preserved disk, not a Firecracker snapshot restore, to avoid snapshot file management and Firecracker version and CPU-template constraints.

## Consequences

- Per-VM state is split: `config.json` (written once at creation, immutable), `state.json` (runtime facts, present only while running) and `stopped.json` (`stopped_at`, written by a deliberate stop and removed on start). `list` compares them with the live process to report `running`, `stopped`, `crashed` (no stop record, process gone) or `legacy`.
- Starting an existing VM reuses its saved configuration; `--mount` and `--env` are rejected. A failed restart rolls back runtime resources only and never deletes the disk.
- A crashed VM is reconciled (leftovers torn down) and then started; its rootfs may be dirty and is not fsck'd.
- Stopped VMs keep their rootfs copy and index, so disk use grows until `delete`. In CNI mode the guest IP may change across restarts.
- VMs created before this change have only `state.json`, are reported as `legacy`, and cannot be restarted. There is no migration.
