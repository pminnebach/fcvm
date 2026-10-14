# 01: Record the VM configuration and report true status

**What to build:** A newly created VM writes an immutable VM configuration record capturing its full effective shape: machine resources and knobs, network mode and index, mounts, env, vsock settings and jailer ids. `list` classifies every VM as `running`, `crashed` or `legacy` using the existing PID-plus-start-time check; a VM with a runtime state record but no configuration record is `legacy`. Stop, cleanup and failed-creation rollback are refactored onto one shared teardown routine with no change in behaviour yet. See the parent spec and `docs/adr/0001-stop-preserves-the-vm.md`.

**Blocked by:** None (can start immediately)

**Status:** ready-for-agent

- [x] Creating a VM writes a configuration record that is never modified afterwards
- [x] The configuration record holds everything needed to boot the VM again without any flags
- [x] `list` shows `running` for a live VM, `crashed` for a dead process with a runtime state record, and `legacy` when there is no configuration record
- [x] A recycled PID is not reported as `running`
- [x] Stop, cleanup and failed-creation rollback use one teardown routine and behave as before
- [x] Manager-level and `list` formatting tests cover each status against a temp state directory
