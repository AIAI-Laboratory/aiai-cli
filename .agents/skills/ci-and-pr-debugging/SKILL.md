---
name: ci-and-pr-debugging
description: >-
  Diagnose GitHub Actions CI failures on PRs, inspect logs of finished jobs while
  sibling matrix jobs are still running, resolve govulncheck stdlib advisories,
  and verify local pre-commit or non-interactive CLI behavior. Use when a PR
  check fails or when running local PR verification in this repository.
---

# CI & PR Verification Runbook

## 1. Inspect Failed Jobs Immediately (Even While Run Is In Progress)

`gh run view <run-id> --job <job-id> --log-failed` fails if any sibling matrix job is still `in_progress`. Fetch the finished job's raw log directly via the GitHub API:

```bash
gh pr checks <pr-number>
gh api --allow-escape-sequences repos/AIAI-Laboratory/aiai-cli/actions/jobs/<job-id>/logs
```

After pushing a fix or retrying a transient runner failure, rerun only failed jobs:

```bash
gh run rerun <run-id> --failed
gh pr checks <pr-number> --watch --interval 15
```

## 2. Triage `CI/quality` Failures

`CI/quality` runs four separate gates (`golangci-lint`, `./scripts/fmt.sh --check`, `govulncheck`, `goreleaser check`). Inspect which gate failed:

- If `golangci-lint` and `fmt.sh --check` pass and `govulncheck` exits with `3` on a `GO-YYYY-NNNN` standard-library advisory (`Found in: ...@goX.Y.Z`, `Fixed in: ...@goX.Y.Z+1`):
  1. Bump the `go` directive in `go.mod` to the fixed patch version.
  2. Update the matching Go version in `README.md` and `docs/CONTRIBUTING.md`.
  3. Commit and push to retrigger CI.

## 3. Local Verification Quirks

- **Git hook `GIT_INDEX_FILE` leakage**: When `.husky/pre-commit` runs inside `git commit`, Git sets `GIT_INDEX_FILE=.git/index`, which breaks temp-repo `git worktree` tests in `internal/git` and `internal/workspace`. Run `go test -race ./...` outside the hook (or with `env -u GIT_INDEX_FILE -u GIT_DIR -u GIT_WORK_TREE`) to confirm tests truly pass.
- **Smoke-testing `aiai init` in a PTY**: Interactive terminals prompt even when `--yes` is passed. Always close stdin (`< /dev/null`) or pass explicit flags (e.g., `--agents-md=append`) when testing non-interactive CLI execution.
