# Jailer isolation knobs

Expose Firecracker jailer features that fcvm does not configure yet, without replacing the default static TAP path.

**Status (verified against `main` on 2026-09-14): Phase 1 and Phase 2 are DONE. Only Phase 3 remains open** — this doc now focuses the active plan on Phase 3.

## Goal

Let operators tighten jailer isolation (cgroups, NUMA, daemonize, netns/CNI, per-VM credentials, PID ns, rlimits) via config. Keep today's TAP + MASQUERADE networking as the default.

## What shipped (Phases 1-2)

**Phase 1 — SDK-exposed jailer knobs:**

- `numa-node`, `daemonize`, `parent-cgroup`, `cgroup` on `config.JailerConfig` (`config/config.go:12-21`), wired into `JailerCfg` via the builder (`vm/fc_config.go:92-107`: `NumaNode`, `Daemonize`, `CgroupArgs`, `ParentCgroup`).
- Unit tests: `vm/fc_config_test.go:59-60,109,134-135`.
- PID tracking with daemonize: `readJailerPIDFile` (`vm/state.go:100-114`), used in `vm/manager.go:278-289`, tested in `vm/jailer_pid_test.go`.
- Docs: `docs/configuration.md:88-91`.

**Phase 2 — Netns / CNI + per-VM credentials:**

- CNI + jailer `--netns`: handled implicitly by firecracker-go-sdk as a side effect of `CNIConfiguration` (see [cni-network.md](done/cni-network.md), now archived as done) — the SDK creates `/var/run/netns/<VMID>` and passes `--netns` to the jailer itself; documented in `docs/network.md:82` and `network/cni.go:17,21`.
- Optional per-VM uid/gid: `config.JailerConfig.PerVMUIDs` (`config/config.go:16`), implemented in `jailerCreds(cfg, index)` (`vm/fc_config.go:148-156`: `if cfg.Jailer.PerVMUIDs { uid += index; gid += index }`), wired at `vm/manager.go:107`, tested at `vm/fc_config_test.go:184`, documented in `fcvm.example.yaml:7` and `docs/configuration.md:87`. Shared-uid risk when left off is documented alongside it.

## What's still open — Phase 3: jailer flags missing from the SDK

`--new-pid-ns` and `--resource-limit` are Firecracker jailer flags that firecracker-go-sdk's `JailerConfig` struct does not expose. Confirmed absent repo-wide (no `NewPidNs`/`NewPIDNs`/`new-pid-ns` or `ResourceLimit`/`resource-limit` anywhere in `config/`, `vm/`, `cmd/`).

### Design

Since the SDK doesn't expose these, they can't be set via `JailerConfig` struct fields. Two implementation paths, in order of preference:

1. **Check for a newer firecracker-go-sdk release that added these fields first.** If the vendored SDK version predates support, check upstream (`go.mod`'s current `firecracker-go-sdk` version) for a release that adds `NewPidNs`/`ResourceLimits` (or similarly named) to `JailerConfig`, and bump the dependency if a compatible version exists. This is strictly simpler than building custom argv construction.
2. **If the SDK genuinely has no path for these flags**, build the jailer command line manually via the SDK's process-runner extension point (`firecracker.WithProcessRunner` or equivalent — check what the vendored SDK version actually exposes for overriding how the jailer binary is invoked) and append `--new-pid-ns` (boolean flag, no value) and `--resource-limit <key>=<value>` (repeatable) to the constructed argv before exec.

### Config sketch

```yaml
jailer:
  new-pid-ns: true
  resource-limits:
    - no-file=1024
    - fsize=250000000
```

- Add `NewPidNS bool` and `ResourceLimits []string` to `config.JailerConfig` (`config/config.go`).
- Defaults: both empty/false — fully backward compatible.
- `Validate()`: loosely validate `resource-limits` entries are `key=value` shaped (Firecracker/jailer defines the allowed keys — pass through rather than hardcoding an allowlist, since the jailer itself will reject bad keys).

### Implementation

- Wherever the jailer command/argv is ultimately constructed (inside the SDK today, or in a new fcvm-side wrapper if path (2) from Design above is needed), append the two flags when configured.
- If a custom argv path is needed, keep it narrowly scoped to just appending these two flags — do not reimplement the SDK's existing jailer-invocation logic.

### Tests

- Unit test asserting the constructed jailer argv includes `--new-pid-ns` when `NewPidNS: true`, and one or more `--resource-limit` entries matching the configured list, when enabled. If flags are set directly on a (possibly upgraded) SDK `JailerConfig` struct, a builder test similar to the existing `fc_config_test.go:101-153` (`TestBuildFirecrackerConfigOverrides`) suffices.

### Documentation

- `docs/configuration.md`: document the new keys, and document PID file location under the jail root specifically when `--new-pid-ns` is set (the PID namespace changes what PID the jailer/Firecracker process appears as from inside vs. outside the namespace — the existing PID-tracking logic (`readJailerPIDFile`, `vm/state.go:100-114`) needs to be re-verified against the jail-root PID file, not a namespace-relative PID, when this flag is on).

## Non-goals

- Do not remove or demote static TAP + iptables MASQUERADE as the default.
- Do not require CNI plugins for the basic `fcvm start` path.
- Do not invent abstractions beyond config → SDK/jailer argv.
- Do not change jailer opt-out (fcvm always uses jailer).

## Success criteria

- `--new-pid-ns` and `--resource-limit` are configurable and verified (via test) to reach the jailer's actual invocation.
- PID tracking continues to work correctly with `--new-pid-ns` enabled.
- `go test ./...` green; no behavior change when the new config keys are unset.
