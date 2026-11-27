---
trigger: model_decision
description: Cross-platform filesystem locking and Git subprocess environment guardrails for Go code in internal/ and cmd/. Apply when modifying file locks, atomic writes, temp files, or Git test/command execution.
---

# Cross-Platform Go & Git Safety Rules

1. **Avoid delete-and-recreate lock files (`O_CREATE|O_EXCL` + `os.Remove`)**:
   - Prefer OS-held locks on an unremoved lock file (the `internal/localdata` pattern using `LockFileEx`/`flock`).
   - If an exclusive-create lock file (`O_CREATE|O_EXCL`) must be used and removed on unlock, handle transient `fs.ErrPermission` (`ERROR_ACCESS_DENIED`) with bounded retries on `runtime.GOOS == "windows"` because NTFS returns `ERROR_ACCESS_DENIED` while an unlinked file is delete-pending.
2. **Isolate Git subprocess tests from parent hook environment**:
   - Git hooks export `GIT_INDEX_FILE`, `GIT_DIR`, and `GIT_WORK_TREE`. Tests or helpers invoking `git` inside temporary repositories (especially `git worktree`) must not rely on inheriting a clean environment without clearing or overriding those variables.
