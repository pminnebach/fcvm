# 06: Crashed VM recovery

**What to build:** `start <id>` and `restart <id>` on a crashed VM first tear down what the crash left behind (TAP device, firewall rules, NFS exports, jailer tree), then boot normally from the preserved disk. `stop <id>` on a crashed VM performs the same leftover teardown and records the stop, so the user can acknowledge the crash. No `fsck` is run.

**Blocked by:** 03

**Status:** ready-for-agent

- [x] `start` on a crashed VM succeeds without running `cleanup` first
- [x] Leftover TAP, rules, exports and jailer tree from the crash are removed before boot
- [x] `stop` on a crashed VM leaves it `stopped` with a stop record and no leftover resources
- [x] A failed recovery leaves the disk and configuration intact
- [x] Manager tests simulate a crash by leaving runtime state and a dead process
