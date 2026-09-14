# Optional CNI networking

**Status: DONE — shipped in v1.1.0 (see [docs/version_history.md](../../docs/version_history.md)).** Verified against `main` on 2026-09-14; every checklist item below is implemented and tested. This doc is kept as the historical design record.

Wire firecracker-go-sdk CNI so operators can set `network.cni-network` and get netns-isolated guest networking. Static TAP + MASQUERADE remains the default when the field is empty.

Related: jailer netns is a side effect of CNI in the SDK — see [jailer-isolation.md](../jailer-isolation.md) phase 2 (CNI details live here).

## Goal

- When `network.cni-network` is non-empty, start the VM with `CNIConfiguration` instead of hand-rolled TAP.
- Persist the resolved guest IP for SSH/`exec`/list.
- Defer NFS mount setup until after CNI assigns addresses.
- Leave the empty-`cni-network` path behaviorally identical to today.

## Non-goals

- Do not remove or demote static TAP as default.
- Do not vendor or install CNI plugins inside fcvm; document host requirements only.
- Do not support multiple guest NICs (SDK limitation with IP config).
- Do not implement jailer cgroup/uid knobs here (that is [jailer-isolation.md](../jailer-isolation.md)).

## Locked decisions

| Topic | Choice |
|-------|--------|
| Activation | `network.cni-network` non-empty → CNI; empty → today's TAP |
| Guest IP | SDK fills from CNI result (`tc-redirect-tap`); written into state after `Start` |
| Rootfs patch | `PatchNetwork` skipped in CNI mode |
| NFS | `SetupNFSExport` + MMDS mount metadata deferred until after `Start`; resolved gateway used as NFS server address |
| Cleanup | No `TeardownTap` / orphan `fcvm-tap-*` for CNI VMs; SDK/`network.TeardownCNI` runs CNI DEL on machine cleanup |
| State | CNI mode recorded (empty `TapDev` + `IsCNI()`); tap-index / expected-guest-IP validation skipped for those VMs |
| Default path | Static TAP unchanged |

## Implementation (verified against code)

| Checklist item | Status | Evidence |
|---|---|---|
| `Start()` branches on non-empty `network.cni-network` | DONE | `vm/manager.go:102` `useCNI := m.cfg.Network.CNINetwork != ""` |
| Skip `PatchNetwork` + `SetupTap` on CNI path; use `CNIConfiguration` | DONE | `vm/fc_config.go:119-129` (`buildNetworkInterfaces`); `vm/manager.go:110,160,173` |
| After `Start`, resolve guest IP/gateway into state; defer NFS/MMDS | DONE | `vm/manager.go:252-261`, `resolveCNIAddrs` at `vm/manager.go:410-428`, `setupMounts` at `vm/manager.go:358-399` |
| Stop/cleanup: no `TeardownTap` for CNI VMs; state validation skips TAP index checks | DONE | `vm/manager.go:581-582`, `failCleanup` at `:180-185`, `vm/state.go:55-57` (`IsCNI`), `:80-90` (`ClaimedIndices`), tested in `vm/index_test.go:56-67` |
| Example yaml + docs/network.md host setup | DONE | `fcvm.example.yaml:25`, `docs/network.md:77-121` |
| Runnable test that CNI branch selects `CNIConfiguration` and skips TAP setup | DONE | `vm/fc_config_test.go:155-172` (`TestBuildFirecrackerConfigCNI`), `:200-225` (`TestTeardownStateSkipsTapForCNI`, `TestStateIsCNI`) |

Supporting pieces beyond the original checklist: `network/cni.go` implements `TeardownCNI` (CNI DEL + netns removal via `containernetworking/cni/libcni`), tested in `network/cni_test.go`. `cmd/experimental.go:36,51-52` and `cmd/root.go:57,78,131` wire `--cni-network` as a real CLI flag gated behind the experimental-confirmation flow, tested in `cmd/experimental_test.go:31-36`.

## Host prerequisites (unchanged, see docs/network.md for full detail)

- Plugins under `/opt/cni/bin`: `ptp`, `host-local`, `firewall`, `tc-redirect-tap`.
- Config under `/etc/cni/conf.d`, e.g. `fcnet.conflist`, whose `name` field must match `network.cni-network`.

## Success criteria (met)

- `fcvm start` with empty `cni-network` unchanged.
- `fcvm start` with `cni-network: fcnet` (and host plugins/conflist) boots, SSH works to CNI-assigned IP, stop cleans via CNI DEL.
- NFS mounts (if configured) work after deferred setup using CNI gateway.
