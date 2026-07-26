# NIC and drive rate limiters

Wire Firecracker token-bucket rate limiters on network interfaces and block drives so a single guest cannot saturate host I/O or bandwidth. Defaults stay unlimited; ship a conservative production example.

## Goal

Expose SDK rate limiter fields via fcvm config and apply them when building `firecracker.Config`. Operators opt in through yaml; everyday debug VMs remain uncapped.

## Symptom

Guests share the host’s NIC and disk with no Firecracker-level throttle. A noisy or hostile VM can starve co-tenants (and the host control plane) for bandwidth or IOPS. Firecracker prod-host-setup recommends configuring rate limiters; fcvm never sets them.

## Root cause

[`vm/fc_config.go`](../vm/fc_config.go) builds drives and `NetworkInterfaces` without `RateLimiter` / `InRateLimiter` / `OutRateLimiter`. The firecracker-go-sdk already provides helpers (`NewRateLimiter`, token bucket models); fcvm simply never passes them.

## Locked decisions

| Topic | Choice |
|-------|--------|
| Surfaces | Guest NIC in/out + each drive (including rootfs) |
| Config | Token buckets in yaml (size + one-time burst, matching SDK/Firecracker semantics) |
| Defaults | Unlimited (nil limiters) — no behavior change for existing users |
| Example | `fcvm.example.yaml` (or a commented production block) ships a conservative profile |
| CNI path | Same limiter fields on the CNI `NetworkInterface` when CNI is used |
| Validation | Reject nonsense values (zero/negative) at config load or start with a clear error |

## Config sketch

```yaml
# production-oriented example — omit entirely for unlimited
network:
  rate-limiter:
    in:
      bandwidth: { size: 10000000, one-time-burst: 10000000, refill-time: 100 }  # bytes / ms per Firecracker docs
      ops:       { size: 1000, one-time-burst: 1000, refill-time: 100 }
    out:
      bandwidth: { size: 10000000, one-time-burst: 10000000, refill-time: 100 }

# optional per-drive; rootfs may share a top-level drive rate-limiter
# drives:
#   rate-limiter: …
```

Exact yaml field names should match existing mapstructure style in [`config/config.go`](../config/config.go); adjust to Firecracker’s `TokenBucket` fields at implement time. Prefer SDK `firecracker.NewRateLimiter` rather than hand-rolling model structs.

## Fix

1. Add config structs for token buckets and optional in/out NIC + drive limiters.
2. In `buildNetworkInterfaces` / drive construction in [`vm/fc_config.go`](../vm/fc_config.go), attach `InRateLimiter`, `OutRateLimiter`, and drive `RateLimiter` when config is set.
3. Document units and point at Firecracker rate-limiter docs.
4. Comment a conservative production profile in example yaml.

## Code touch list

| Area | Change |
|------|--------|
| [`config/config.go`](../config/config.go) | Structs + mapstructure tags |
| [`vm/fc_config.go`](../vm/fc_config.go) | Wire SDK limiters |
| [`fcvm.example.yaml`](../fcvm.example.yaml) | Production example block |
| [`docs/configuration.md`](../docs/configuration.md) | Field table + units |
| Tests in `vm/` or `config/` | Unmarshal + assert non-nil limiters on built config when set; nil when unset |

## Check to leave behind

Unit test: with empty config, built `NetworkInterfaces` and drives have nil rate limiters; with a filled production-style config, `InRateLimiter`/`OutRateLimiter` (and drive limiter) are non-nil and carry the configured bucket sizes. No Firecracker process required.

## Non-goals

- Do not enable limiters by default.
- Do not invent host-level `tc` / cgroup I/O rules here (cgroups are [jailer-isolation.md](jailer-isolation.md)).
- Do not auto-tune buckets from host capacity.

## Success criteria

- Optional yaml applies NIC and drive rate limiters through the SDK.
- Default start remains unlimited.
- Example production profile is copy-pasteable and documented.
