# 07: Mounts and guest IP across restarts

**What to build:** Writable block mount images are reused when an existing VM starts and are not rebuilt from the host directory, so guest-side changes are not overwritten; `stop` still syncs them back to the host directory. NFS exports are recreated on every start against the VM's current guest IP. In TAP mode the guest IP is stable across restarts; in CNI mode it is re-resolved at each start. `list` shows a non-running VM's guest IP only in TAP mode and `-` in CNI mode.

**Blocked by:** 03

**Status:** ready-for-agent

- [x] A block mount image with guest-side changes survives stop and start unchanged
- [x] `stop` still mirrors writable block mounts to the host directory
- [x] NFS exports are recreated on start and scoped to the guest IP resolved for that boot
- [x] In CNI mode a changed guest IP leads to exports for the new IP
- [x] `list` shows the guest IP of a stopped TAP VM and `-` for a stopped CNI VM
- [x] Manager and `list` formatting tests cover the above
