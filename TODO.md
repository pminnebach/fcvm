# TODO

## Correctness and data safety

- [ ] Fix VM index allocation — index reuse lets a new VM delete a running VM's TAP and steal its IP (see [plans/vm-index-allocation.md](plans/vm-index-allocation.md))
- [ ] Stop losing guest writes on block-fallback mounts; make the fallback opt-in and sync back on stop (see [plans/mount-writeback.md](plans/mount-writeback.md)) — absorbs the former "sync block-fallback mount images back to the host directory on stop" item
- [ ] Validate VM ids before they reach `os.RemoveAll` as root, and guard the remaining mount/`RemoveAll` pairs (see [plans/destructive-path-guards.md](plans/destructive-path-guards.md))
- [ ] Make `stop` PID-safe and give `list` a real status column (see [plans/vm-liveness.md](plans/vm-liveness.md))

## Security

- [ ] Scope NFS exports to the guest instead of `*`, and move staging out of `/tmp` (see [plans/nfs-export-hardening.md](plans/nfs-export-hardening.md))
- [ ] Stop flipping the host's `FORWARD` policy and `ip_forward`; use a dedicated chain and clean up on teardown (see [plans/host-network-scope.md](plans/host-network-scope.md))
- [ ] Verify downloaded assets before executing them as root; add HTTP timeouts (see [plans/asset-integrity.md](plans/asset-integrity.md))

## CLI and guest

- [ ] CLI ergonomics: context/signal handling, `SilenceUsage`, strict mount parsing, `exec` argument quoting (see [plans/cli-ergonomics.md](plans/cli-ergonomics.md))
- [ ] Guest bootstrap: configurable DNS, drop shell JSON parsing, quote-safe env injection (see [plans/guest-bootstrap.md](plans/guest-bootstrap.md))
- [ ] Interactive serial console via screen (see [plans/serial-console.md](plans/serial-console.md))
- [ ] Expose unused Firecracker jailer isolation knobs (see [plans/jailer-isolation.md](plans/jailer-isolation.md))
- [ ] Implement optional CNI networking (see [plans/cni-network.md](plans/cni-network.md))

## Hygiene

- [ ] Add CI (gofmt, vet, test, build) — three files are currently unformatted on `main` (see [plans/repo-hygiene.md](plans/repo-hygiene.md))
- [ ] Delete dead code, fix always-nil error and the `docker create` panic, repoint the fake network-config test, stop root tests from mutating the host (see [plans/repo-hygiene.md](plans/repo-hygiene.md))

## Done

- [x] Add parameter to change rootfs size at build.
- [x] Rewrite docs into docs/ (architecture, network, kernel, rootfs, install, cli, configuration) and slim README.
- [x] Add architecture / config / ~/.fcvm layout docs (see docs/).
- [x] Add version command in the application.
- [x] Fix mounted folder being emptied upon microvm crash or running "fcvm cleanup [--all]"
- [x] Expose-kvm doesn't do anything. KVM always works in a microvm with a kvm enabled kernel.

## Suggested order

The first three correctness items are small, self-contained diffs and each has an obvious test; do them first. Then the NFS export scope and the iptables chain, which are the two changes a user is most likely to be bitten by without noticing. Add CI before the hygiene batch so the cleanup stays clean.

Note the cross-references: [mount-writeback.md](plans/mount-writeback.md), [destructive-path-guards.md](plans/destructive-path-guards.md), and [cli-ergonomics.md](plans/cli-ergonomics.md) all touch `mountFlag`, and [host-network-scope.md](plans/host-network-scope.md) subsumes the `defaultIface` cleanup listed in [repo-hygiene.md](plans/repo-hygiene.md).
