# Jailer isolation knobs

Expose Firecracker jailer features that fcvm does not configure yet, without replacing the default static TAP path.

## Goal

Let operators tighten jailer isolation (cgroups, NUMA, daemonize, netns/CNI, per-VM credentials, PID ns, rlimits) via config. Keep today’s TAP + MASQUERADE networking as the default.

## Current vs desired

**Today** ([vm/manager.go](../vm/manager.go)):

- Always starts via jailer with `JailerBinary`, `UID`/`GID`, `NumaNode=0` (hardcoded), auto `CgroupVersion`, `ChrootBaseDir`, `NaiveChrootStrategy`.
- Config surface ([config/config.go](../config/config.go)): `jailer.uid`, `jailer.gid`, `jailer.chroot-base-dir` only.
- Networking is hand-rolled TAP ([network/tap.go](../network/tap.go)); `network.cni-network` is reserved and unused ([fcvm.example.yaml](../fcvm.example.yaml)).
- Jailer always does mount ns + chroot, drops privileges, creates `/dev/kvm` and `/dev/net/tun`. No `--netns`.

**Desired:** phased config knobs mapped to firecracker-go-sdk `JailerConfig` / `Config.NetNS` where possible; custom process runner only for flags the SDK does not expose.

## Config sketch

```yaml
jailer:
  chroot-base-dir: ~/.fcvm/jailer
  uid: 1000          # shared default; phase 2 can allocate per-VM
  gid: 1000
  numa-node: 0       # phase 1; stop hardcoding
  daemonize: false   # phase 1
  parent-cgroup: ""  # phase 1
  cgroup:            # phase 1 → JailerCfg.CgroupArgs
    - memory.max=1G
  # phase 2
  # per-vm-uids: true   # or uid-base / range — pick one shape at implement time
  # phase 3 (custom argv if SDK still lacks fields)
  # new-pid-ns: true
  # resource-limits:
  #   - no-file=1024
  #   - fsize=250000000

network:
  tap-ip: 172.16.0.1
  guest-ip: 172.16.0.2
  # cni-network: fcnet   # see plans/cni-network.md
```

Defaults stay backward-compatible: unset new fields behave as today.

**Daemonize (phase 1):** confirm PID tracking still works (`machine.PID()` / `firecracker.pid` in jail root) before enabling by default — leave default `false`.

CNI / netns start-stop details: [cni-network.md](cni-network.md).

## Phased checklist

### Phase 1 — SDK-exposed jailer knobs

- [ ] Add `numa-node`, `daemonize`, `parent-cgroup`, `cgroup` to `config.JailerConfig` + viper defaults / example yaml.
- [ ] Wire into `JailerCfg` in `vm/manager.go` (replace hardcoded `numa := 0`).
- [ ] Unit test: config unmarshaling; optional assert that `JailerCfg` fields are set when starting with a fake/mocked path if practical.
- [ ] Docs: short note in [docs/network.md](../docs/network.md) or a jailer subsection in README — only what phase 1 exposes.

### Phase 2 — Netns / CNI + per-VM credentials

- [ ] CNI + jailer `--netns`: implement per [cni-network.md](cni-network.md) (single source of truth).
- [ ] Optional per-VM uid/gid (Firecracker prod-host-setup recommendation); document shared-uid risk if left off.

### Phase 3 — Jailer flags missing from SDK

- [ ] `--new-pid-ns` and `--resource-limit`: either upstream firecracker-go-sdk `JailerConfig` fields, or build jailer argv via `WithProcessRunner` / `JailerCommandBuilder` extension.
- [ ] One small self-check or test that the constructed argv includes the flags when enabled.
- [ ] Document PID file location under jail root when using `--new-pid-ns`.

## Non-goals

- Do not remove or demote static TAP + iptables MASQUERADE as the default.
- Do not require CNI plugins for the basic `fcvm start` path.
- Do not invent abstractions beyond config → SDK/jailer argv.
- Do not change jailer opt-out (fcvm always uses jailer).
