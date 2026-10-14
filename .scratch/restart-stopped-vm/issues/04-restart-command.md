# 04: `restart <id>`

**What to build:** A `restart` command. On a running VM it stops then starts it; on a stopped or crashed VM it just starts it. An unknown id is a "no such VM" error and `restart` never creates a VM.

**Blocked by:** 03

**Status:** ready-for-agent

- [x] `restart` on a running VM leaves it running again with its disk preserved
- [x] `restart` on a stopped VM starts it
- [x] `restart` on an unknown id fails with a "no such VM" error and creates nothing
- [x] `--mount` and `--env` are not accepted by `restart`
- [x] Manager and cmd tests cover running, stopped and unknown ids
