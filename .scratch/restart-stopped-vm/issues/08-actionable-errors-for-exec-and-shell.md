# 08: Actionable errors for `exec` and `shell`

**What to build:** `exec` and `shell` (and `vsock-exec`) on a VM that is not running report its status (`stopped`, `crashed` or `legacy`) and say how to start it, instead of a connection failure.

**Blocked by:** 02

**Status:** ready-for-agent

- [ ] `exec` and `shell` on a stopped VM name the status and suggest `start <id>`
- [ ] The same holds for a crashed VM
- [ ] A legacy VM's message says it cannot be restarted and suggests `cleanup`
- [ ] An unknown id still reports that the VM does not exist
- [ ] cmd tests cover each status
