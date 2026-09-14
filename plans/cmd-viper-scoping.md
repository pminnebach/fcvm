# Move `cmd/` off the global viper singleton

## Problem

`cmd/root.go` and other `cmd/*.go` files call package-level `viper.BindPFlag`, `viper.SetDefault`, `viper.SetEnvPrefix`, `viper.AutomaticEnv`, `viper.ReadInConfig`, `viper.Unmarshal` — all against viper's global default instance, not a scoped `*viper.Viper` created via `viper.New()`. Since the singleton is process-global, any test that invokes cobra command setup more than once in the same test binary (or in parallel) risks config values leaking between test cases, and there is no isolation between successive `fcvm` invocations within the same process.

## Goal

Replace the global viper singleton with a `*viper.Viper` instance scoped to each root command invocation (or at least to each test), so config state cannot leak between calls in-process. Preserve all existing behavior for the built `fcvm` binary — this is purely an internal-state change with no user-visible config/flag/env behavior change.

## Design

1. Create a `*viper.Viper` (via `viper.New()`) once per root-command construction. Check the current structure of `cmd/root.go` first — if the command tree is built at package `init()` time via a package-level `rootCmd`, refactor to a constructor function (e.g. `NewRootCmd() *cobra.Command`) that creates both the command tree and its scoped viper instance together, called once from `main.go` and once per test. This is the standard cobra+viper testability pattern.
2. Thread that instance through to every place that currently calls a package-level `viper.*` function: `BindPFlag`, `SetDefault`, `SetEnvPrefix`, `AutomaticEnv`, `SetConfigFile`/`ReadInConfig`, `Unmarshal`.
3. `loadConfig()` (referenced in `cmd/root.go`'s `PersistentPreRunE`) needs the scoped instance passed in (closure, or a small `app`/`cmdContext` struct field) instead of reading the global.
4. Keep `cmd.OutOrStdout()` usage as-is — that groundwork is already done and orthogonal to this change.

## Files likely touched

- `cmd/root.go` — the bulk of the change: viper instance creation, `loadConfig`, `PersistentPreRunE`.
- Any other `cmd/*.go` file that calls `viper.*` directly (`grep -rn 'viper\.' cmd/` first to get the full list — based on the audit, `root.go` is the primary site, but double-check `start.go`, `experimental.go`, etc. for direct calls).
- `cmd/root_test.go` (and any new test files) — add a test that constructs two root commands in the same test process with different config, asserting no leakage.

## Tests

- New test: build two `NewRootCmd()` instances in sequence within one test function, set different config values via flags/env on each, assert the second doesn't see the first's values. Write this test first against the current code to confirm the bug is real/reproducible before refactoring.
- Full `go test ./cmd/...` and `go test ./...` to confirm no behavior change for the built binary path.
- Manual smoke: build `fcvm`, run with `--config`, flags, and env vars in the usual combinations, confirm identical behavior to before.

## Non-goals

- Do not change any user-visible config precedence (flag > env > config file > default) or key names.
- Do not introduce a DI framework or restructure `cmd/` package layout beyond what's needed for the scoped viper instance.
- Do not touch `assets/`, `vm/`, or other packages — this is `cmd/`-internal.

## Success criteria

- No package-level `viper.*` calls remain in `cmd/` (grep confirms only scoped-instance calls).
- New leakage-regression test passes.
- `go test ./...` and a manual binary smoke test show no behavior change.
