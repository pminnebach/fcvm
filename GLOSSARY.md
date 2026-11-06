# fcvm

A root-required Linux CLI that manages the lifecycle of Firecracker microVMs: create, run, stop, restart and delete them, and run commands inside.

## Language

**VM**:
A Firecracker microVM managed by fcvm, identified by a validated id. It owns a disk, a network slot and its mounts and env.
_Avoid_: sandbox, microVM, Firecracker VM

**Stopped VM**:
A VM that was deliberately stopped by the user: its process is not running, but its disk and configuration are kept so it can be started again.
_Avoid_: paused, suspended

**Crashed VM**:
A VM whose process is gone although the user never stopped it. Its disk and configuration are kept like a stopped VM, and it can be started again.
_Avoid_: dead, failed, orphan

**Incomplete VM**:
A VM whose creation never finished: it has a configuration but never booted and was never stopped, so its disk may be half built. It cannot be started, only deleted.
_Avoid_: broken, partial

**VM configuration**:
The shape a VM was created with (resources, mounts, env, network mode). It is fixed at creation and reused on every start.
_Avoid_: settings, spec

**Deleted VM**:
A VM whose disk, configuration and reserved slot have all been removed. It cannot be started again.
_Avoid_: destroyed, cleaned up

**Start**:
Boot a VM from its disk. For a stopped VM this is a cold boot of the preserved disk, with no memory state restored.
_Avoid_: resume, restore

**Restart**:
Stop a running VM and start it again, or start it if it is already stopped.
_Avoid_: reboot, recreate

**Config file**:
A YAML file fcvm reads its settings from: the User config, the Directory config, or the one named by `--config`.
_Avoid_: settings file, rc file

**User config**:
`~/.fcvm.yaml` in the invoking user's home (the `SUDO_USER` home under sudo). The lowest-precedence Config file.
_Avoid_: global config, home config

**Directory config**:
`./.fcvm.yaml` in the exact current directory, never its parents. It is layered over the User config, and must be trusted (owned by the invoking user or root, not group- or world-writable) because fcvm runs as root.
_Avoid_: local config, project config
