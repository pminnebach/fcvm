# 09: Docs and release notes

**What to build:** Bring the documentation in line with the new lifecycle: the on-disk layout and the stop and cleanup sections in the architecture doc, the command reference for `start <id>`, `restart`, `delete`, `stop` and the new `list` statuses, and a release note. The note says `stop` no longer deletes, `delete` reclaims disk space, and VMs created before this change show as `legacy` and cannot be restarted.

**Blocked by:** 04, 05, 06, 07, 08

**Status:** ready-for-agent

- [ ] The architecture doc describes the configuration, runtime state and stop records and the new stop, start-existing and delete flows
- [ ] The CLI reference documents `restart`, `delete`, `start <id>` and the `list` statuses and columns
- [ ] A release note covers the behaviour change of `stop` and the legacy break
- [ ] The manual smoke test is run on real hardware and its result noted: write a file in the guest, `stop`, `start`, confirm the file and (TAP) the IP survive; kill Firecracker and confirm `crashed` then recovery with `start`
