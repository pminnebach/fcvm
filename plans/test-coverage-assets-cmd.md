# Improve test coverage in `assets/` and `cmd/`

## Current state (measured 2026-09-14)

- `assets/`: 39.9% statement coverage. 3 non-test files (`download.go` 427 lines, `kernel_ci.go` 126 lines, `patch.go` 35 lines) vs. 2 test files (`download_test.go`, `kernel_ci_test.go`). **`patch.go` has no test file at all.**
- `cmd/`: 31.3% statement coverage. 12 non-test files vs. only 2 test files (`root_test.go` — pure `mountFlag` helper only, `experimental_test.go`). **10 of 12 command files have zero dedicated tests**: `start.go`, `stop.go`, `exec.go`, `vsock_exec.go`, `list.go`, `cleanup.go`, `build_rootfs.go`, `download.go`, `self_check.go`, `version.go`.

This continues the coverage gap first flagged in [plans/done/repo-hygiene.md](done/repo-hygiene.md) (where `assets/` was 14.6% and `cmd/` was 22%) — improved since, but still the thinnest-covered packages relative to `vm/`/`network/`.

## Goal

Add focused unit tests for the highest-value untested surfaces, without requiring root/KVM/a real Firecracker binary — cobra command tests should test flag parsing, validation, and error paths, not actually spin up VMs (that's what `vm` package tests + manual smoke testing are for).

## Priority list

### `assets/patch.go` (0% coverage — highest priority, smallest file)

Test `PatchExt4` and `cleanupMountTemp` with a small fixture. Check the pattern already used in `download_test.go`/`kernel_ci_test.go` (likely avoids real mounts to run without root) and follow the same approach for whatever `PatchExt4` actually needs mocked/faked.

### `cmd/*.go` — flag parsing and validation (no VM startup)

For each of the 10 untested files, focus tests on:
- Flag/argument validation that returns an error before any VM/state/network side effect (e.g. `start.go`'s flag-combination validation beyond the already-tested `mountFlag` helper).
- Help text and usage string generation (cheap, catches regressions like the `--mount` `auto` omission noted in [plans/code-review-fixes.md](code-review-fixes.md)).
- Error paths that don't require root: `stop.go`/`cleanup.go`/`list.go` against a missing/malformed state directory; `exec.go`/`vsock_exec.go` argument validation (VM id required, command required) before any SSH/vsock connection is attempted; `self_check.go`'s pure checks that don't need real hardware; `version.go`'s output format.
- Use the cobra test pattern already established in `cmd/experimental_test.go` (construct the command, set args via `SetArgs`, capture output via `cmd.SetOut`/`cmd.SetErr`, consistent with the existing `cmd.OutOrStdout()` groundwork) as the template for all of these.

### `assets/download.go`, `assets/kernel_ci.go` (already partially covered — extend, don't duplicate)

Check what `download_test.go`/`kernel_ci_test.go` already cover (likely the `httptest`-based cases from `plans/done/asset-integrity.md`) before adding more — fill gaps rather than re-testing the same paths. Lower priority than the two items above.

## Non-goals

- No coverage percentage gate added to CI (matches the explicit non-goal already stated in [plans/done/repo-hygiene.md](done/repo-hygiene.md)) — this is about closing specific, valuable gaps, not chasing a number.
- Do not write tests that require root, KVM, or a real Firecracker binary — those belong in manual smoke-testing procedures, not `go test ./...`.
- Do not restructure `cmd/`'s command construction just to make testing easier unless [plans/cmd-viper-scoping.md](cmd-viper-scoping.md) is implemented first (its constructor-function refactor would make per-command testing considerably easier — consider sequencing that plan before this one).

## Verification

- `go test ./assets/... ./cmd/... -cover` before and after, confirming coverage increases and no existing test breaks.
- `go test ./...` full run stays green.
