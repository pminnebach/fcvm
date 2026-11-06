# Release notes

## v1.4.0 (2026-11-06)

Directory config release: a `.fcvm.yaml` in the directory you run fcvm from is now layered over your `~/.fcvm.yaml`, so each project can carry its own values. Broken config files are now errors instead of being silently ignored, and shell completion no longer breaks on them. Earlier versions: [version history](https://github.com/pminnebach/fcvm/blob/main/docs/version_history.md).

### Behaviour change

- The Directory config (`./.fcvm.yaml`) now wins over the User config (`~/.fcvm.yaml`), and the User config is layered underneath instead of ignored. Before, when both files existed only the User config was read and the Directory config was silently ignored. Keys the Directory config leaves out still come from the User config, then the defaults. `env:` merges per variable (Directory config wins, casing kept), lists such as `network.nameservers` are replaced whole, and `--config <path>` still reads only that file. See [configuration.md](https://github.com/pminnebach/fcvm/blob/main/docs/configuration.md)
- A config file that exists but cannot be used is now an error naming the file and the reason, for a YAML syntax error, a wrong-typed value such as `vcpu-count: "four"`, or a `.fcvm.yaml` that is a directory or a dangling symlink. Before, fcvm silently ran with the defaults. A missing file is still not an error, except for an explicit `--config`

### New

- Only the exact current directory is checked for a Directory config, never its parents. When the current directory is your home directory, `~/.fcvm.yaml` is read once, as the User config
- Because fcvm runs as root and a config file chooses which binaries it runs, a Directory config is only trusted when it is owned by the invoking user (the `SUDO_USER` user under sudo) or by root, and is not group- or world-writable. Otherwise the command fails naming the file and the reason. The User config and `--config` are not checked. An invalid `SUDO_UID` is an error
- A Directory config may not set `state-dir`, so every directory sees the same VMs and two state directories never hand the same network slot to two running VMs. `state-dir` still works from flags, `FCVM_*` environment variables, the User config and `--config`
- Each config file read is logged at debug level with its role (User config, Directory config or explicit), and `fcvm self-check` prints a `config files:` line, or `none`, before its `/dev/kvm` and root checks

### Fixes

- Shell tab-completion no longer loads config. Before, a bad value in a config file broke every `<TAB>` press. `completion` and its subcommands, `__complete`, `version`, `help` and `experimental` never resolve config, so they work whatever the config files contain

### Upgrade notes

- If you already had both a `~/.fcvm.yaml` and a `./.fcvm.yaml`, values in the Directory config now take effect where they were ignored before
- A config file with a typo that used to be ignored now stops the command; fix the file it names
- A VM's shape still comes from its saved configuration. Starting a stopped VM from another directory does not change it; binaries, kernel, rootfs and timeouts follow the current layered config, as before
