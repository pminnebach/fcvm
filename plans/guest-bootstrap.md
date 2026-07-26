# Guest bootstrap

The injected guest scripts hardcode DNS, re-parse JSON with `grep`/`sed`, and build shell `export` lines by concatenation. The host already knows all of this at patch time.

## Issue 1 — DNS is hardcoded to 8.8.8.8

Two places set it, both in the start path:

- `buildNetworkInterfaces` ([vm/fc_config.go](../vm/fc_config.go)) sets `Nameservers: []string{"8.8.8.8"}`.
- `startScript` ([rootfs/hooks.go](../rootfs/hooks.go)) runs `echo nameserver 8.8.8.8 > /etc/resolv.conf` on **every** boot, so it also clobbers anything the guest image or an operator configured.

This sends guest lookups to Google by default, and breaks air-gapped hosts, split-horizon DNS, and any environment with an internal resolver.

**Fix:** add a `network.nameservers` config list. Default to the host's resolvers read from `/etc/resolv.conf` (falling back to the current value only if that yields nothing), and pass the configured list into the NIC config and into the injected script. Make the guest script write `/etc/resolv.conf` only when the file is absent or when fcvm previously wrote it — a marker comment line is enough to tell the difference.

## Issue 2 — Guest scripts re-parse MMDS JSON in shell

`applyMountsScript` ([rootfs/hooks.go](../rootfs/hooks.go)) reconstructs mount records from JSON with line-window heuristics:

```sh
chunk=$(grep -B5 "\"guest\"[[:space:]]*:[[:space:]]*\"$gp\"" /tmp/fcvm-mounts.json | tail -6)
method=$(echo "$chunk" | sed -n 's/.*"method"[^"]*"\([^"]*\)".*/\1/p' | head -1)
```

This depends on the field order and the exact line spacing that `SetMetadata` happens to emit, and it silently does the wrong thing rather than failing when the shape changes. Block device mapping is worse — it assumes drive order maps to `/dev/vdb`, `/dev/vdc`, `/dev/vdd` and hardcodes exactly three, so a fourth mount is silently ignored.

`initEnvScript` has the same class of parser (`grep -o` for `"k":"v"` pairs), already marked `ponytail: naive KEY=VAL parse`.

**Fix:** stop parsing JSON in the guest. The host knows the mount list and env at patch time, and already mounts the rootfs to inject hooks — so write a plain, line-oriented file (`/etc/fcvm/mounts`, tab- or NUL-separated fields) during `PatchExt4`, and reduce the guest script to a `while read` loop. MMDS stays for anything genuinely dynamic, but neither mounts nor env is dynamic after boot.

The block-device mapping should be explicit rather than positional: the host chooses the drive IDs in `buildFirecrackerConfig`, so it can record the expected device path per mount in the same file.

If MMDS must remain the transport for some case, install a JSON parser in the rootfs and use it — but a file written by the same code that mounts the image is strictly less machinery.

## Issue 3 — Env injection is quote-unsafe

```sh
echo "export ${key}=\"${val}\"" >> /etc/fcvm/env
```

`/etc/fcvm/env` is then sourced by `/etc/profile.d/fcvm.sh`. A value containing `"`, `\`, backtick, or `$(...)` breaks the file or executes in the guest shell. The values come from the operator's own `--env` flags, so this is a correctness bug rather than a privilege boundary — but it silently corrupts ordinary values like passwords and JSON blobs.

**Fix:** write the env file from the host with proper single-quote escaping (`'` → `'\''`), as part of the same patch-time file write as Issue 2. That deletes the guest-side parser entirely.

## Issue 4 — Dead injected script

`fcvm-mounts.sh` (`mountsScript` in [rootfs/hooks.go](../rootfs/hooks.go)) fetches the mounts JSON and then does nothing with it — its only body is a comment pointing at `fcvm-start.sh`. It is written into every guest image and called by nothing. Delete it.

## Locked decisions

| Topic | Choice |
|-------|--------|
| DNS default | Host `/etc/resolv.conf` nameservers; configurable via `network.nameservers` |
| resolv.conf | Written only if absent or previously fcvm-written (marker comment) |
| Mounts/env transport | Host-written files under `/etc/fcvm/`, injected at rootfs patch time |
| MMDS | Kept available, no longer the source for mounts/env |
| Block devices | Explicit device path per mount recorded by the host, not positional |
| Escaping | Single-quote escaping done in Go, not in shell |
| Dead script | `fcvm-mounts.sh` removed |

## Code touch list

| Area | Change |
|------|--------|
| [rootfs/hooks.go](../rootfs/hooks.go) | Write `/etc/fcvm/mounts` + `/etc/fcvm/env` from Go; shrink guest scripts to `read` loops; delete `mountsScript`; conditional resolv.conf |
| [assets/patch.go](../assets/patch.go) | Pass mounts/env through to the patch step |
| [vm/manager.go](../vm/manager.go) | Supply the resolved mount list (including device paths) at patch time |
| [vm/fc_config.go](../vm/fc_config.go) | Nameservers from config |
| [config/config.go](../config/config.go) | `network.nameservers` |
| [docs/rootfs.md](../docs/rootfs.md), [docs/network.md](../docs/network.md) | Document the new guest files and DNS behaviour |

## Check to leave behind

The escaping is the part that will break quietly, and it is pure string work: a Go test that renders the env file for values containing `"`, `'`, `$`, a backslash, and a newline, then asserts the output round-trips — ideally by running `sh -c '. file; printf %s "$KEY"'` in the test, which is a real check and still needs no VM.

Extend the existing `TestInjectHooksSystemdUnit` ([rootfs/hooks_test.go](../rootfs/hooks_test.go)) to assert `fcvm-mounts.sh` is no longer written.

## Non-goals

- No guest agent daemon or vsock control channel; SSH stays the control plane.
- Do not require `jq` or any other package in the guest image.
- Do not change the systemd-unit-plus-rc.local dual bootstrap.
- No DHCP in the guest.

## Success criteria

- A guest on an air-gapped host resolves via the host's resolvers, and a guest image that ships its own `/etc/resolv.conf` keeps it.
- An env value containing quotes and `$` arrives in the guest byte-for-byte.
- Four or more block mounts all get mounted at the right paths.
- No JSON is parsed by shell in the guest, and `fcvm-mounts.sh` is gone.
