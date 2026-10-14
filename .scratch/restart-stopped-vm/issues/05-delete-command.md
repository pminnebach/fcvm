# 05: `delete <id>`

**What to build:** A `delete` command, the deliberate way to remove a VM. It removes the disk, block mount images, configuration record, jailer tree, any leftovers, and frees the reserved network slot. It refuses a running VM unless `--force`, which stops the VM first. An unknown id is an error. `cleanup` keeps its best-effort orphan-reclaim role and shares the same teardown. Every deletion path keeps the existing id validation and mount/`RemoveAll` guards.

**Blocked by:** 02

**Status:** ready-for-agent

- [ ] `delete` on a stopped VM removes everything and frees its index for reuse
- [ ] `delete` on a running VM is refused without `--force`
- [ ] `delete --force` on a running VM stops then deletes it
- [ ] `delete` on an unknown id fails clearly
- [ ] `cleanup` and `cleanup --all` still reclaim orphans with or without records
- [ ] Ids are validated before use as path components on the new path
- [ ] Manager and cmd tests cover stopped, running, forced, unknown and orphan cases
