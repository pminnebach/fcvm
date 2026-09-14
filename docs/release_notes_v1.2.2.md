# Release notes

## v1.2.2 (2026-09-14)

Patch release: fixes uppercase environment variable names from `.fcvm.yaml` getting lowercased in the guest. Earlier versions: [version history](https://github.com/pminnebach/fcvm/blob/main/docs/version_history.md).

### Fixes

- Env vars configured under `env:` in `.fcvm.yaml` now keep their original casing in the guest. Viper lowercases every key it reads from the config file, including map values nested under `env:`, so `MY_VAR: value` was previously injected into the guest as `my_var`. Variables passed via `--env` on the CLI were unaffected since that path bypasses viper. `loadConfig` now re-reads the config file's `env:` section directly with a case-preserving YAML parser instead of relying on viper's lowercased copy.
