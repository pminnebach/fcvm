# Optional CNI networking

Wire firecracker-go-sdk CNI so operators can set `network.cni-network` and get netns-isolated guest networking. Keep static TAP + MASQUERADE as the default when the field is empty.

Related: jailer netns is a side effect of CNI in the SDK — see [jailer-isolation.md](jailer-isolation.md) phase 2 (CNI details live here).

## Goal

- When `network.cni-network` is non-empty, start the VM with `CNIConfiguration` instead of hand-rolled TAP.
- Persist the resolved guest IP for SSH/`exec`/list.
- Defer NFS mount setup until after CNI assigns addresses.
- Leave the empty-`cni-network` path behaviorally identical to today.

## Non-goals

- Do not remove or demote static TAP as default.
- Do not vendor or install CNI plugins inside fcvm; document host requirements only.
- Do not support multiple guest NICs (SDK limitation with IP config).
- Do not implement jailer cgroup/uid knobs here (that is [jailer-isolation.md](jailer-isolation.md)).

## Locked decisions

| Topic | Choice |
|-------|--------|
| Activation | `network.cni-network` non-empty → CNI; empty → today’s TAP |
| Guest IP | SDK fills from CNI result (`tc-redirect-tap`); write into `state.GuestIP` after `Start` |
| Rootfs patch | Skip `PatchNetwork` in CNI mode; rely on SDK `ip=` kernel config (guest hooks no-op if `/etc/fcvm/network` is missing) |
| NFS | Defer `SetupNFSExport` + MMDS mount metadata until after `Start`; use resolved gateway as NFS server address |
| Cleanup | No `TeardownTap` / orphan `fcvm-tap-*` for CNI VMs; SDK runs CNI DEL on machine cleanup |
| State | Record CNI mode (e.g. `network_mode: "cni"` or empty `tap_dev`); skip tap-index / expected-guest-IP validation for those VMs |
| Default path | Static TAP unchanged |

## Current flow

Today ([vm/manager.go](../vm/manager.go)):

1. Allocate VM index → `tapIP`/`guestIP` via `SubnetForIndex`, `tapDev` via `TapDevName`.
2. `assets.PatchNetwork(rootfs, guestIP, tapIP)`.
3. `network.SetupTap` (TAP + ip_forward + shared MASQUERADE).
4. NFS exports using `guestIP` / mount metadata `tapIP:exportPath`.
5. `NetworkInterfaces` with `StaticConfiguration` (MAC, `HostDevName`, IP/gateway/DNS).
6. On stop: `TeardownTap(state.TapDev)`.

`config.NetworkConfig.CNINetwork` already exists and is unused ([config/config.go](../config/config.go), [fcvm.example.yaml](../fcvm.example.yaml)).

## CNI flow

```mermaid
flowchart TD
  start[fcvm start]
  mode{cni-network set?}
  tapPath[PatchNetwork SetupTap StaticConfiguration]
  cniCfg[CNIConfiguration NetworkName]
  boot[NewMachine Start]
  resolve[Read guest IP gateway from iface StaticConfiguration]
  nfs[Deferred SetupNFSExport SetMetadata mounts]
  save[SaveState GuestIP network_mode]
  stop[fcvm stop]
  tearTap[TeardownTap]
  tearCni[Machine cleanup CNI DEL]

  start --> mode
  mode -->|empty| tapPath --> boot --> save
  mode -->|set| cniCfg --> boot --> resolve --> nfs --> save
  stop --> mode
  mode -->|empty| tearTap
  mode -->|set| tearCni
```

### Start (CNI branch)

1. Skip `PatchNetwork`, `SetupTap`, and static IP/MAC derivation from config tap/guest base.
2. Build:

```go
NetworkInterfaces: []firecracker.NetworkInterface{{
  CNIConfiguration: &firecracker.CNIConfiguration{
    NetworkName: m.cfg.Network.CNINetwork, // e.g. "fcnet"
    IfName:      "veth0",
    VMIfName:    "eth0",
  },
  AllowMMDS: true,
}}
```

3. SDK creates `/var/run/netns/<VMID>`, runs CNI ADD, fills `StaticConfiguration` from `tc-redirect-tap` result, passes `--netns` to jailer.
4. After `machine.Start`: read guest IP and gateway from the resolved interface config.
5. Run deferred NFS setup with that guest IP; MMDS mount `host` = `gateway:exportPath`.
6. `SaveState` with `GuestIP`, `network_mode: "cni"` (or empty `TapDev`), no `fcvm-tap-N` reclaim responsibility.

### Stop / cleanup (CNI branch)

- Do not call `TeardownTap`.
- Let SDK machine stop/cleanup invoke CNI DEL.
- `cleanup --all` orphan TAP reclaim stays TAP-only (ignore CNI VMs).

## Host prerequisites

Document in [docs/NETWORK.md](../docs/NETWORK.md):

- Plugins under `/opt/cni/bin`: `ptp`, `host-local`, `firewall`, `tc-redirect-tap` (and any others the conflist needs).
- Config under `/etc/cni/conf.d`, e.g. `fcnet.conflist`:

```json
{
  "name": "fcnet",
  "cniVersion": "0.3.1",
  "plugins": [
    {
      "type": "ptp",
      "ipMasq": true,
      "ipam": {
        "type": "host-local",
        "subnet": "192.168.127.0/24",
        "resolvConf": "/etc/resolv.conf"
      }
    },
    { "type": "firewall" },
    { "type": "tc-redirect-tap" }
  ]
}
```

- `network.cni-network` must match the conflist `name` field.

## Code touch list

| Area | Change |
|------|--------|
| [vm/manager.go](../vm/manager.go) | Branch on `CNINetwork`; skip TAP/patch; CNI iface; post-Start IP + deferred NFS; stop teardown branch |
| [vm/state.go](../vm/state.go) | `NetworkMode` (or equivalent); skip tap/guest consistency checks for CNI |
| [config/config.go](../config/config.go) / [cmd/root.go](../cmd/root.go) | Field already exists; bind/document if needed |
| [fcvm.example.yaml](../fcvm.example.yaml) | Uncomment/document `cni-network` as optional CNI mode |
| [docs/NETWORK.md](../docs/NETWORK.md) | CNI mode section, sample conflist, plugin deps |
| Tests | One unit test: building start config with CNI set yields `CNIConfiguration` and does not call `SetupTap` (extract iface-building helper if that keeps the test small) |

## Checklist

- [ ] Branch `Start` on non-empty `network.cni-network`.
- [ ] Skip `PatchNetwork` + `SetupTap` on CNI path; use `CNIConfiguration`.
- [ ] After `Start`, resolve guest IP/gateway into state and deferred NFS/MMDS.
- [ ] Stop/cleanup: no `TeardownTap` for CNI VMs; state validation skips TAP index checks.
- [ ] Example yaml + NETWORK.md host setup.
- [ ] One runnable test that CNI branch selects `CNIConfiguration` and skips TAP setup.

## Success criteria

- `fcvm start` with empty `cni-network` unchanged.
- `fcvm start` with `cni-network: fcnet` (and host plugins/conflist) boots, SSH works to CNI-assigned IP, stop cleans via CNI DEL.
- NFS mounts (if configured) work after deferred setup using CNI gateway.
