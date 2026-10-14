# TODO

## Open

- [ ] Interactive serial console via screen (see [plans/serial-console.md](plans/serial-console.md))
- [ ] Expose remaining Firecracker jailer isolation knobs — `--new-pid-ns` / `--resource-limit` (Phase 3; Phases 1-2 are done) (see [plans/jailer-isolation.md](plans/jailer-isolation.md))
- [ ] Optional Firecracker features from firectl: initrd, general vsock device config, metrics FIFO, user extra drives, root partuuid (see [plans/optional-firecracker-features.md](plans/optional-firecracker-features.md))
- [ ] Graceful Firecracker API shutdown before SIGTERM/Kill on stop (see [plans/graceful-stop.md](plans/graceful-stop.md))
- [ ] Fix outstanding code-review bugs: SSH warning noise, completion skip-list, CNI empty-gateway NFS mis-scoping, dead `--` handling, vsock-exec socket race, install-kasm.sh debug logging, no build/download progress feedback (see [plans/code-review-fixes.md](plans/code-review-fixes.md))
- [ ] Repo hygiene follow-up: delete dead `DownloadKernel` wrapper, fix stale Dockerfile references in docs/rootfs.md (see [plans/repo-hygiene-followup.md](plans/repo-hygiene-followup.md))

### Follow-ups from the review

- [ ] Move `cmd/` off the global viper singleton so config does not leak between tests (the `cmd.OutOrStdout()` groundwork is done) (see [plans/cmd-viper-scoping.md](plans/cmd-viper-scoping.md))
- [ ] In-process SSH client via `x/crypto/ssh` instead of shelling out to `ssh` (see [plans/inprocess-ssh-client.md](plans/inprocess-ssh-client.md))
- [ ] `copyFile` is not sparse-aware, so every `start` writes the full rootfs size (see [plans/sparse-copy.md](plans/sparse-copy.md))
- [ ] Coverage is still thin in `assets/` (39.9%) and `cmd/` (31.3%) (see [plans/test-coverage-assets-cmd.md](plans/test-coverage-assets-cmd.md))

## Done

- [x] Add parameter to change rootfs size at build.
- [x] Rewrite docs into docs/ (architecture, network, kernel, rootfs, install, cli, configuration) and slim README.
- [x] Add architecture / config / ~/.fcvm layout docs (see docs/).
- [x] Add version command in the application.
- [x] Fix mounted folder being emptied upon microvm crash or running "fcvm cleanup [--all]"
- [x] Expose-kvm doesn't do anything. KVM always works in a microvm with a kvm enabled kernel.
- [x] Fix VM index allocation — index reuse let a new VM delete a running VM's TAP and steal its IP ([plans/done/vm-index-allocation.md](plans/done/vm-index-allocation.md))
- [x] Stop losing guest writes on block-fallback mounts; the fallback is now opt-in and syncs back on stop ([plans/done/mount-writeback.md](plans/done/mount-writeback.md))
- [x] Validate VM ids before they reach `os.RemoveAll` as root, and guard the remaining mount/`RemoveAll` pairs ([plans/done/destructive-path-guards.md](plans/done/destructive-path-guards.md))
- [x] Make `stop` PID-safe and give `list` a real status column ([plans/done/vm-liveness.md](plans/done/vm-liveness.md))
- [x] Scope NFS exports to the guest instead of `*`, and move staging out of `/tmp` ([plans/done/nfs-export-hardening.md](plans/done/nfs-export-hardening.md))
- [x] Stop flipping the host's `FORWARD` policy and `ip_forward`; use a dedicated chain and clean up on teardown ([plans/done/host-network-scope.md](plans/done/host-network-scope.md))
- [x] Verify downloaded assets before executing them as root; add HTTP timeouts ([plans/done/asset-integrity.md](plans/done/asset-integrity.md))
- [x] CLI ergonomics: context/signal handling, `SilenceUsage`, strict mount parsing, `exec` argument quoting ([plans/done/cli-ergonomics.md](plans/done/cli-ergonomics.md))
- [x] Guest bootstrap: configurable DNS, no shell JSON parsing, quote-safe env injection ([plans/done/guest-bootstrap.md](plans/done/guest-bootstrap.md))
- [x] Add CI (gofmt, vet, test, build) ([plans/done/repo-hygiene.md](plans/done/repo-hygiene.md))
- [x] Delete dead code, fix always-nil error and the `docker create` panic, repoint the fake network-config test, stop root tests from mutating the host ([plans/done/repo-hygiene.md](plans/done/repo-hygiene.md))
- [x] Auto-rebase default TAP `/30` on host collision; hard-error for explicit colliding bases ([plans/done/host-subnet-collision.md](plans/done/host-subnet-collision.md))
- [x] Implement optional CNI networking ([plans/done/cni-network.md](plans/done/cni-network.md))
- [x] Adopt firectl-inspired config improvements: pure `firecracker.Config` builder, machine knobs (kernel-args/log-level/cpu-template/disable-smt), version command prints supported Firecracker version, jailer phase 1-2 config (numa/daemonize/cgroup, per-VM uids) ([plans/done/firectl-lessons.md](plans/done/firectl-lessons.md))
