# Release notes

## v1.3.1 (2026-11-26)

Patch release: in TAP mode, the TAP device is now owned by the VM's jailer user, so the jailed Firecracker can attach to it on Linux 6.5 and later. Earlier versions: [version history](https://github.com/pminnebach/fcvm/blob/main/docs/version_history.md).

### Fixes

- On hosts running Linux 6.5 or later, a VM in TAP mode could fail to start because Firecracker was not allowed to attach to its TAP device. Firecracker attaches (`TUNSETIFF`) after the jailer has dropped to the VM's uid/gid, and since Linux 6.5 ("tun: fix group permission check") a TAP with no owner or group can only be attached with `CAP_NET_ADMIN`. fcvm now creates the device with `ip tuntap add ... user <uid> group <gid>` using the VM's jailer uid/gid. CNI mode is unaffected

### Security

- Because the TAP device now has an owner, other unprivileged local users can no longer attach to a VM's TAP device

### Upgrade notes

- The TAP device is created at each `start`, so existing VMs pick up the fix the next time they are started or restarted. A VM that is running during the upgrade keeps its ownerless device until then
