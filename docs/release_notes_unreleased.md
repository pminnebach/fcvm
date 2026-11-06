# Release notes

## Unreleased

### Behaviour change

- The Directory config (`./.fcvm.yaml`) now wins over the User config (`~/.fcvm.yaml`), and the User config is layered underneath instead of ignored. Before, when both files existed only the User config was read and the Directory config was silently ignored. Keys the Directory config leaves out still come from the User config, then the defaults. `env:` merges per variable (Directory config wins, casing kept), lists such as `network.nameservers` are replaced whole, and `--config <path>` still reads only that file. See [configuration.md](configuration.md)
