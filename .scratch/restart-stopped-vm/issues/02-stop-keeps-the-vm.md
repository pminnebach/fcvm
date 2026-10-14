# 02: `stop` keeps the VM

**What to build:** `stop` becomes non-destructive. It signals the process as today, syncs writable block mounts back to the host directory, releases TAP/CNI, firewall rules, NFS exports and the jailer tree, writes a stop record with the stop time, and removes the runtime state record last. The disk, block images, configuration record and network index are kept. `stop` on an already stopped VM succeeds. New-VM index allocation never reuses a stopped VM's index. The host firewall chain and `ip_forward` are released when the last running VM stops, not when the last VM on disk goes. `list` shows `stopped` with a stopped-at column.

**Blocked by:** 01

**Status:** ready-for-agent

- [x] After `stop`, the VM's disk, configuration and index are still on disk
- [x] `list` shows the VM as `stopped` with its stop time, and a VM with no stop record whose process died still shows `crashed`
- [x] The stop record is written before the runtime state record is removed, so an interrupted stop shows as `crashed`
- [x] A new VM never takes a stopped VM's network index or guest IP
- [x] A stopped VM holds no TAP device, firewall rules, NFS exports or jailer tree
- [x] The host firewall chain and `ip_forward` are restored when the last running VM stops, even if stopped VMs remain
- [x] `stop` on an already stopped VM succeeds, and on an unknown id fails clearly
- [x] Manager tests cover the above against a temp state directory with host commands recorded
