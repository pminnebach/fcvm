# Repo hygiene follow-up: last items from `plans/done/repo-hygiene.md`

Source: [plans/done/repo-hygiene.md](done/repo-hygiene.md) is otherwise fully resolved (gofmt clean, CI gofmt/vet/test/build steps in place, `buildFirecrackerConfig`/`docker.go` fixes landed, `vm/network_config_test.go` now tests the real builder, `network.run` has an injectable seam used by tests, `docs/architecture.md` doc-drift fixed). One dead-code item from its table was never removed, plus one small new doc-drift item surfaced while verifying `docs/review.md` item 4.

## 1. Delete the dead `DownloadKernel` wrapper

**File:** `assets/download.go`, around lines 257-259.
**Problem:** `func DownloadKernel(...) error { return DownloadFile(...) }` is a one-line pass-through with no distinct signature or behavior — the original repo-hygiene review flagged it for deletion, conditional on it not gaining a distinct signature via `plans/done/asset-integrity.md`. That condition never materialized; it's still a pure pass-through.
**Fix:** delete `DownloadKernel`; update any call sites to call `DownloadFile` directly (`grep -rn 'DownloadKernel' assets/ cmd/` first to find them all).
**Verify:** `go build ./...`, `go test ./assets/... ./cmd/...`.

## 2. Fix stale Dockerfile references in `docs/rootfs.md`

**File:** `docs/rootfs.md` (around line 12).
**Problem:** still lists `Dockerfile.Ubuntu-2604` and `Dockerfile.Kasm` as "Example Dockerfiles in the repo," but both were deleted when `Dockerfile.Default` was simplified to `FROM ubuntu:26.04` directly (git log: "Update rootfs", "Clenaup and simplify dockerfiles for rootfs", 2026-09-14). Only `Dockerfile` and `Dockerfile.Default` remain.
**Fix:** update the Dockerfile listing in `docs/rootfs.md` to match the current repo contents (`Dockerfile`, `Dockerfile.Default`); remove any now-inaccurate two-step base-image build instructions.
**Verify:** `ls Dockerfile*` and diff against what `docs/rootfs.md` claims; doc-only change, no code impact.

## Non-goals

- Do not re-open any other item from `plans/done/repo-hygiene.md` — everything else there is confirmed done.
- Do not touch `Dockerfile`/`Dockerfile.Default` themselves — this is docs-only plus one dead-code deletion.

## Success criteria

- `grep -rn DownloadKernel assets/ cmd/` returns nothing (or only non-definition references are gone).
- `docs/rootfs.md` accurately lists the Dockerfiles that exist in the repo.
