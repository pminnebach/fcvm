# Jailer isolation knobs

Expose Firecracker jailer features that fcvm does not configure yet, without replacing the default static TAP path. Also covers production-host gaps that belong here (resource limits, recommended cgroups, per-VM uids) rather than separate plan files.

## Goal

Let operators tighten jailer isolation (cgroups, NUMA, daemonize, netns/CNI, per-VM credentials, PID ns, rlimits) via config. Keep today’s TAP + MASQUERADE networking as the default. Document a stock cgroup recipe and treat `per-vm-uids: true` as the expected production setting without flipping the compatibility default in-code until an explicit later decision.

## Current vs desired

**Today** ([vm/manager.go](../vm/manager.go)):

- Always starts via jailer with `JailerBinary`, `UID`/`GID`, `NumaNode` from config, auto `CgroupVersion`, `ChrootBaseDir`, `NaiveChrootStrategy`.
- Config surface ([config/config.go](../config/config.go)): `jailer.uid`, `jailer.gid`, `jailer.chroot-base-dir`, plus phase-1 fields as implemented (`numa-node`, `daemonize`, `parent-cgroup`, `cgroup`).
- Networking is hand-rolled TAP ([network/tap.go](../network/tap.go)); optional CNI when configured.
- Jailer always does mount ns + chroot, drops privileges, creates `/dev/kvm` and `/dev/net/tun`.
- Production gaps still open: no `--resource-limit` / `--new-pid-ns` in the argv we build; empty/absent cgroup recipe in examples; shared uid 1000 by default (`per-vm-uids` not the documented production expectation).

**Desired:** phased config knobs mapped to firecracker-go-sdk `JailerConfig` / `Config.NetNS` where possible; custom process runner only for flags the SDK does not expose. Example yaml and docs steer production toward per-VM uids + a stock cgroup recipe without silently forcing them on every start.

## Config sketch

```yaml
jailer:
  chroot-base-dir: ~/.fcvm/jailer
  uid: 1000          # shared default (compat); production: prefer per-vm-uids
  gid: 1000
  numa-node: 0
  daemonize: false
  parent-cgroup: ""
  # Recommended stock recipe for production (not applied unless set):
  cgroup:
    - memory.max=1G
    - cpu.max=100000 50ms   # adjust to host; example only — lock exact strings at implement/docs time
    - io.weight=100         # cgroup v2 names; v1 hosts need the matching keys
  per-vm-uids: false        # default stays false for backward compatibility
  # production docs/examples: per-vm-uids: true
  # phase 3 (custom argv — SDK JailerConfig has no these fields today)
  new-pid-ns: false
  resource-limits:
    - no-file=1024
    - fsize=250000000

network:
  tap-ip: 172.16.0.1
  guest-ip: 172.16.0.2
  # cni-network: fcnet   # see plans/cni-network.md
```

Defaults stay backward-compatible: unset new fields behave as today.

**Daemonize (phase 1):** confirm PID tracking still works (`machine.PID()` / `firecracker.pid` in jail root) before enabling by default — leave default `false`.

**Per-VM uids (phase 2 / production guidance):** keep code default `per-vm-uids: false` (or absent ≡ false) so existing single-uid setups do not break. Multi-VM and production docs + `fcvm.example.yaml` should present `per-vm-uids: true` as the expected setting. A major-version default flip is a separate explicit decision later — this plan is “docs + example first.”

**Cgroups:** ship a recommended stock `jailer.cgroup` recipe in example yaml/docs (memory / cpu / blkio or cgroup-v2 equivalents). Do **not** silently inject that recipe on every start when the operator left `cgroup` empty.

CNI / netns start-stop details: [cni-network.md](cni-network.md).

Path trustworthiness of bins/chroot-base: [jailer-path-hardening.md](jailer-path-hardening.md) (separate plan).

## Phased checklist

### Phase 1 — SDK-exposed jailer knobs

- [ ] Add `numa-node`, `daemonize`, `parent-cgroup`, `cgroup` to `config.JailerConfig` + viper defaults / example yaml (if not already present).
- [ ] Wire into `JailerCfg` in `vm/manager.go` / `vm/fc_config.go` (no hardcoded NUMA).
- [ ] Unit test: config unmarshaling; optional assert that `JailerCfg` fields are set when starting with a fake/mocked path if practical.
- [ ] Docs: short note in [docs/network.md](../docs/network.md) or a jailer subsection in README — only what phase 1 exposes.
- [ ] Docs/example: include the **recommended stock cgroup recipe** as comments or a production block; leave runtime default empty/unset.

### Phase 2 — Netns / CNI + per-VM credentials

- [ ] CNI + jailer `--netns`: implement per [cni-network.md](cni-network.md) (single source of truth).
- [ ] Optional per-VM uid/gid (Firecracker prod-host-setup recommendation); document shared-uid risk if left off.
- [ ] Production docs + example yaml: treat `per-vm-uids: true` as the expected multi-VM / production setting; keep compatibility default `false` unless a later major-version decision flips it.

### Phase 3 — Jailer flags missing from SDK

firecracker-go-sdk `JailerConfig` (see upstream `jailer.go`) does **not** expose `--resource-limit` or `--new-pid-ns` today. Do not pretend the SDK fields exist.

- [ ] `--new-pid-ns` and `--resource-limit`: build jailer argv via `WithProcessRunner` / `JailerCommandBuilder` extension (or equivalent custom argv), not nonexistent SDK struct fields.
- [ ] One small self-check or test that the constructed argv includes the flags when enabled.
- [ ] Document PID file location under jail root when using `--new-pid-ns`.
- [ ] Example yaml: sample `resource-limits` list aligned with Firecracker prod-host-setup.

## Non-goals

- Do not remove or demote static TAP + iptables MASQUERADE as the default.
- Do not require CNI plugins for the basic `fcvm start` path.
- Do not invent abstractions beyond config → SDK/jailer argv.
- Do not change jailer opt-out (fcvm always uses jailer).
- Do not silently force stock cgroups or `per-vm-uids: true` on every start in this plan’s compatibility window.
- Do not duplicate path-permission checks here ([jailer-path-hardening.md](jailer-path-hardening.md)).
