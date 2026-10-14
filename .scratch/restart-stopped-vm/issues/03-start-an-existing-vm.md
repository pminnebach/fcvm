# 03: `start <id>` boots an existing VM

**What to build:** `start <id>` on a stopped VM boots it from its preserved disk using its saved configuration, reusing its network index and guest identity, then clears the stop record. `--mount` and `--env` on an existing VM fail with an error telling the user to delete and recreate. Starting a running VM fails clearly. `start` with a new id or no id still creates a VM exactly as today. If a restart fails partway, only the runtime resources acquired in that attempt are released; the VM stays stopped with its disk and configuration intact. A failed creation still removes everything it created.

**Blocked by:** 02

**Status:** ready-for-agent

- [x] A stopped VM starts from its preserved disk and files written before the stop are still there
- [x] The restarted VM has the same configuration and, in TAP mode, the same guest IP
- [x] The stop record is removed once the VM is running
- [x] `--mount` or `--env` on an existing VM is rejected with a message explaining how to get a different shape
- [x] Starting a running VM fails without side effects
- [x] A failed restart leaves the VM `stopped` with its disk and configuration untouched
- [x] A failed creation still removes everything it created
- [x] Manager tests (Firecracker launch stubbed) and cmd tests for the flag rules cover the above
