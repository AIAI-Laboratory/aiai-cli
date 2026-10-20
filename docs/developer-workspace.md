# Developer Workspace implementation report

Implemented locally on 2026-10-20. No commit, push, release tag, or global binary
replacement is included in this change.

## Delivered features and commands

- Git: `git`, `git status`, `git changes`, `git diff [--interactive]`,
  `git history`, `git branches`, `git worktrees`, `git activity`, `git board`,
  and `git repo add/list/remove`. Read-only, offline, no GitHub authentication.
- Workspace: `workspace`, `workspace add/list/show/remove/board/today/activity/path`.
  Explicit registration for Git, non-Git and linked-worktree contexts; unavailable
  directories retain their metadata and saved focus records.
- Zen: the existing aquarium/timer is retained, with a five-option landing menu,
  `--project`, `--repo`, `--label`, project/repository-filtered stats and history,
  active-transition persistence and interrupted-session recovery.
- Integration: Home `/workspace` and `/git`, asynchronous local context, sequential
  terminal ownership, project focus from the board, and source-identified,
  chronological Git/focus timelines with date/branch/project filters.
- TUI: responsive 120×40, 100×30, 80×24, 60×20 and 40×15 layouts, smaller-terminal
  fallback, selected-file diff, staged toggle, independent preview scrolling,
  explicit loading/error/empty states, search, contextual help and NO_COLOR.

See the [README](../README.md#developer-workspace) for complete usage and key bindings.

## Architecture and storage decisions

The existing frameworks, main menu, SSH/auth/updater/scaffolder and monitor are
preserved. No new module dependency was needed. Following the minimal Ponytail
approach, Git uses installed `git` through `os/exec`, Zen reuses its existing
aquarium/state machine, and Workspace projects live in a separate JSON registry
rather than a database. Presentation never performs Git/filesystem operations in
`View()`. Expensive queries run as cancellable Bubble Tea commands; multi-project
queries have four bounded workers.

`internal/localdata` supplies atomic/private JSON writes and OS-held locks on Unix
and Windows. `workspaces.json` is schema 1; `zen.json` is schema 2 with explicit
schema-1 compatibility/migration. Unknown schemas/corruption are preserved, not
reset. Git data remains live and is not copied into these stores. Timers persist
before optional Git enrichment; a failed Git observation cannot lose focus data.

Home no longer checks updates during startup: `/update` remains available as an
explicit online action. The startup context query is local and does not register
the current directory.

## Files created

- `internal/git/service.go`, `service_test.go`: bounded subprocess runner, status,
  history, branches, worktrees, diffs, activity and snapshot comparison.
- `internal/workspace/service.go`, `service_test.go`: registry, project overview,
  bounded board collection, session association and timeline.
- `internal/localdata/file.go`, `file_test.go`, `lock_unix.go`, `lock_windows.go`:
  shared versioned JSON persistence and cross-process OS locks.
- `internal/tui/developer/model.go`, `model_test.go`: shared Git/Workspace TUI.
- `internal/command/developer.go`, `developer_test.go`: CLI commands and routing.
- `internal/zen/recovery_test.go`: migration, owner recovery, ordering and clocks.
- This implementation report.

## Files updated or replaced

- `internal/app/dependencies.go`: inject Git/Workspace services.
- `internal/command/root.go`, `zen.go`: command registration, JSON fields,
  project focus, filtering and sequential TUI routing.
- `internal/output/json.go`: additive Git/Workspace envelope fields.
- `internal/tui/model.go`, `update.go`, `keymap.go`, `view.go`,
  `screens/home.go`: Home entry points, context and offline startup.
- `internal/tui/zen/model.go`, `model_test.go`: stable session IDs, transition
  writes, Git context, background timer observation, lifecycle and layout fixes.
- `internal/zen/model.go`, `repository.go`, `timer.go`, `statistics.go`: associated
  sessions, idempotent upserts, recovery, sleep/resume and DST/average fixes.
- The old `internal/zen/replace_unix.go` and `replace_windows.go` are removed;
  their replacement behavior is shared in `internal/localdata`.
- `README.md`: commands, navigation, security, storage and known limits.

## Verification

- `go test ./...`: passed, including existing regression tests.
- `go test -race ./...`: passed.
- `go vet ./...`: passed.
- golangci-lint v2.15.0: zero issues.
- `scripts/fmt.sh --check` and `git diff --check`: passed.
- CGO-disabled cross-builds: darwin, linux and windows, each amd64 and arm64.
- Actual runtime testing: macOS arm64 only. Linux, Windows and macOS amd64 were
  cross-compiled, not executed; this report does not claim runtime validation there.
- Local Git fixtures cover unborn/clean/modified repositories, staging, Unicode,
  newline paths, renames, conflicts, detached/locked worktrees, shared identity,
  local ahead/behind information, binary diffs, external-diff suppression,
  missing Git, cancellation, filtering and rewritten-history freshness.
- Workspace/Zen fixtures cover duplicate registrations, missing directories,
  metadata-only removal, concurrent updates, invalid schemas, migration, live/dead
  owners, stale writes, timeline ordering/deduplication, DST, paused/uncertain
  clocks, focus/break transitions and responsive layouts.
- PTY smoke test using isolated `/tmp` configuration: register this repository,
  open Workspace and timeline, run a two-second project-associated focus session,
  quit, restart the CLI and inspect the persisted completed session through JSON.
  Project/branch/HEAD snapshots and 2000 ms actual active time were retained.

## Known limits and follow-ups

### TUI readability pass

The presentation now shares one contrast palette and keyboard-hint style across
Home/SSH, Developer Workspace, Zen and Monitor. Red is reserved for selection,
conflicts and high resource usage; headings use the terminal's normal foreground.
Git Status is split into labeled Working tree, Tracking and Latest commit blocks,
with short commit IDs. Branches, worktrees and activity use spaced contextual
records instead of raw Git output. Workspace columns have explicit dividers;
timeline entries group by date and retain source labels. Developer and Zen footers
stay fixed, with compact hints for smaller terminals. Zen statistics/history/help
can scroll; the approved large clock glyphs and timer behavior are retained.
SSH key fingerprints remain available in details but no longer crowd every list
row. Monitor graphs use semantic colors instead of making every metric red.

Regression checks cover all six Git pages and all Zen screens at 120×40, 100×30,
80×24, 60×20 and 40×15 where applicable, as well as existing Home/SSH/wizard views.
No dependency or data schema changes were required. Terminal font size remains a
terminal preference; AIAI does not modify it.

### Data and platform limits

- Git history is deliberately bounded: 100 commits in the TUI, CLI limit 1–1000
  with date/author/branch filters. There is no infinite-scroll history pagination.
- Ahead/behind is local tracking-ref information. No background remote refresh,
  Git write operation, code editor, staging or discard action is implemented.
- Crash recovery retains only the last meaningful persisted transition; it may
  undercount time since that transition. It never guesses that an abandoned
  session completed. Apparently live/reused process IDs are treated conservatively.
- Focus totals group full active duration by the local session start date, rather
  than splitting individual sessions across midnight.
- Moved projects show unavailable; path editing/automatic relocation is not part
  of this release. Unregister/re-register explicitly; existing global Zen history
  remains stored. JSON files are limited to 32 MiB and need explicit archival if
  they reach that size.
- Git start/end snapshots do not monitor every intermediate branch change and
  cannot prove commit authorship, effort or causality. A revision's reachable
  history is not the same as the branch where a commit was originally made.
- Remaining validation: native Windows/Linux terminal, locking and sleep/wake QA.
  These environments were not available for runtime testing in this session.
