# Repo hygiene, CI, and test quality

Non-behavioural cleanup. Everything here is small; the CI item is the one that stops the rest from coming back.

## 1. There is no CI

No `.github/workflows` exists, despite `.cursor/rules/build-verify.mdc` requiring `go test ./...` and `go build` before a change is considered done. The immediate consequence is visible on `main` right now:

```
$ gofmt -l .
config/config.go
rootfs/hooks.go
vm/fc_config_test.go
```

**Fix:** one workflow on push and pull request that runs `gofmt -l .` (failing if the output is non-empty), `go vet ./...`, `go test ./...`, and `go build -buildvcs=false -o fcvm .`. Keep it to a single Linux job on the Go version in `go.mod`. Adding `golangci-lint` is tempting but a bigger commitment — `staticcheck` alone would catch the empty `if` body noted below, so consider it a follow-up rather than part of this change.

Run `gofmt -w` on the three files as a separate first commit so the formatting noise does not obscure the workflow diff.

## 2. Dead code to delete

| What | Where | Note |
|------|-------|------|
| `MetadataJSON` | [vm/manager.go](../../vm/manager.go) | Exported "helper for tests"; no test calls it |
| `Config.VMID` | [config/config.go](../../config/config.go) | `mapstructure:"-"`, never read or written |
| `InjectHooks` wrapper | [assets/patch.go](../../assets/patch.go) | One-line pass-through to `rootfs.InjectHooks` |
| `DownloadKernel` | [assets/download.go](../../assets/download.go) | Pass-through to `DownloadFile` (keep if [asset-integrity.md](asset-integrity.md) gives it a distinct signature) |
| Empty `if` body | [network/tap.go](../../network/tap.go) | `if err := run("ip","link","del",…); err != nil && … { }` — removed outright by [host-network-scope.md](host-network-scope.md) |
| Redundant teardown call | [network/nfs.go](../../network/nfs.go) | `TeardownNFSExportsForVM` calls `TeardownNFSExport(vmID)` again after the loop already matched it |
| `mountsScript` | [rootfs/hooks.go](../../rootfs/hooks.go) | Covered by [guest-bootstrap.md](guest-bootstrap.md) |

## 3. Small correctness and simplification items

- **`buildFirecrackerConfig` returns an always-nil error** ([vm/fc_config.go](../../vm/fc_config.go)). Drop the error from the signature; the caller's error branch is unreachable. If [vm-index-allocation.md](vm-index-allocation.md) or [mount-writeback.md](mount-writeback.md) introduces a real failure mode here, keep it — check before deleting.
- **`docker.go` can panic on empty output** ([rootfs/docker.go](../../rootfs/docker.go)): `cid := string(cidOut[:len(cidOut)-1])` panics with an out-of-range slice if `docker create` prints nothing. `strings.TrimSpace(string(cidOut))` is the same length and correct.
- **`defaultIface` string-scans JSON** ([network/tap.go](../../network/tap.go)) — fixed as part of [host-network-scope.md](host-network-scope.md), listed here so it is not lost if that plan is deferred.
- **Hardcoded image sizes**: `syncDirToExt4` uses 512M ([vm/manager.go](../../vm/manager.go)) and `SquashfsToExt4` uses 1G ([assets/download.go](../../assets/download.go)), while `build-rootfs` already exposes `--size`. The mount case is handled by [mount-writeback.md](mount-writeback.md); the squashfs case needs the same treatment (size from source, or a flag).
- **`copyFile` ignores source permissions and is not sparse-aware** ([vm/manager.go](../../vm/manager.go)). Every `start` copies the full rootfs image; `io.Copy`/`ReadFrom` writes zeros for holes, so a sparse 4G image becomes 4G on disk. Worth a look if start latency or disk use becomes a complaint — not urgent.

## 4. Test quality

**`vm/network_config_test.go` tests a copy, not the code.** `fcvmNetworkConfig` hand-builds a `firecracker.Config` that duplicates `buildNetworkInterfaces`, then asserts the SDK's `ValidateNetwork` accepts it. A regression in the real builder would not fail this test. Point it at `buildFirecrackerConfig` — the fixtures in `vm/fc_config_test.go` already show how to call it — and delete the duplicate.

**Root-gated tests mutate the developer's host.** `TestCleanupAllRemovesOrphans` ([vm/cleanup_test.go](../../vm/cleanup_test.go)) calls `Cleanup(true, "")` as root, which reaches `TeardownTap` (`ip link del`), `TeardownNFSExportsForVM` (deletes `/etc/exports.d/fcvm-*.exports` and runs `exportfs -ra`), and `removeJailerTree`. Anyone running `sudo go test ./...` has their host's exports reloaded. `removeExportDirWith` ([network/nfs.go](../../network/nfs.go)) already demonstrates the fix — take the effectful function as a parameter — so give `network.run` the same seam and inject a recorder in tests. That converts "did it wreck the host" into "did it issue the right commands", which is the more useful assertion anyway.

**Coverage gaps** worth one test each, in priority order: `guest/` is at 0% (`LoadOrCreateKey` round-trip is pure filesystem work and easy to test), `assets/` at 14.6% (the `httptest` case in [asset-integrity.md](asset-integrity.md) covers the important half), `cmd/` at 22% (unblocked by the `cmd.OutOrStdout()` change in [cli-ergonomics.md](cli-ergonomics.md)).

## 5. Documentation drift

- [docs/architecture.md](../../docs/architecture.md) shows the jailer layout as `jailer/<id>/...`, but the code builds `jailer/firecracker/<id>/root` (`Manager.jailerTreeDir` and the `chrootDir` in `Start`).
- The `state.json` field list in the same document predates `network_mode`, `cni_network`, `jailer_uid`/`jailer_gid`, and will need `index` and the liveness fields from the plans above.
- Neither [docs/network.md](../../docs/network.md) nor the README mentions that fcvm exports mounts to `*` or flips the host's `FORWARD` policy. Those become accurate once [nfs-export-hardening.md](nfs-export-hardening.md) and [host-network-scope.md](host-network-scope.md) land — update the docs in those changes rather than documenting the current behaviour now.

## Checklist

- [ ] `gofmt -w` the three flagged files (standalone commit).
- [ ] Add the CI workflow; confirm it fails on an intentionally unformatted file, then passes.
- [ ] Delete the dead code in the table above.
- [ ] `buildFirecrackerConfig` signature; `docker.go` `TrimSpace`.
- [ ] Repoint `vm/network_config_test.go` at the real builder.
- [ ] Injection seam for `network.run`; assert commands instead of executing them.
- [ ] Fix the two doc-drift items in `docs/architecture.md`.

## Non-goals

- No module restructuring (`internal/`, package moves).
- No dependency upgrades or `golangci-lint` adoption in this pass.
- Do not delete pre-existing dead code beyond the itemised list.
- No coverage threshold gate in CI.

## Success criteria

- CI is green on `main` and red on an unformatted or failing change.
- `gofmt -l .` is empty.
- `sudo go test ./...` does not modify the host's network or NFS configuration.
- A deliberate break in `buildFirecrackerConfig`'s network wiring fails a test.
