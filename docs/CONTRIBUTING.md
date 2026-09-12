# AIAI CLI Contributing Guide & Git Flow Workflow

Thank you for your interest in contributing to **AIAI CLI**! This document outlines our development process, branching model (Git Flow), coding standards, and step-by-step workflow for proposing changes.

---

## Table of Contents

- [Core Principles](#core-principles)
- [Git Flow Branching Strategy](#git-flow-branching-strategy)
  - [Branch Hierarchy & Overview](#branch-hierarchy--overview)
  - [Branch Types and Rules](#branch-types-and-rules)
- [Branch Naming Conventions](#branch-naming-conventions)
- [Commit Message Conventions](#commit-message-conventions)
- [Development Workflow (Step-by-Step)](#development-workflow-step-by-step)
  - [1. Prerequisites & Environment Setup](#1-prerequisites--environment-setup)
  - [2. Starting a New Task](#2-starting-a-new-task)
  - [3. Local Development & Verification](#3-local-development--verification)
  - [4. Committing Changes](#4-committing-changes)
  - [5. Keeping Your Branch Updated](#5-keeping-your-branch-updated)
  - [6. Opening a Pull Request](#6-opening-a-pull-request)
  - [7. Code Review & Merge](#7-code-review--merge)
- [Release Process](#release-process)
- [Hotfix Process (Emergency Fixes)](#hotfix-process-emergency-fixes)
- [Pull Request Checklist](#pull-request-checklist)

---

## Core Principles

AIAI CLI is built with deterministic guarantees and an offline-first philosophy:
1. **Deterministic Scaffolding**: Template generation must be reproducible. Files are strictly validated against embedded manifests.
2. **Offline-First**: Binary operates entirely offline without background daemons, network calls during scaffolding, or external plugin runners.
3. **Plan → Apply Safety**: File operations are planned, verified against filesystem collision states, and atomically executed.
4. **Zero-Flake Testing**: All tests must pass with `-race` enabled on Linux, macOS, and Windows.

---

## Git Flow Branching Strategy

We follow a standardized **Git Flow** branching model to maintain stability on production while enabling rapid, structured feature development.

### Branch Hierarchy & Overview

```text
main (v1.0.0) ──────────────────────────●───────────────● (v1.1.0)
     ▲                                   │               ▲
     │                                   │ hotfix/*      │ release/*
     │                                   ▼               │
develop ──────●────────●─────────────────●───────────────●
              │        ▲                 ▲
              │        │ feature/*       │ bugfix/*
              ▼        │                 │
           feature/login                 bugfix/preview-overflow
```

### Branch Types and Rules

| Branch | Purpose | Base Branch | Merges Into | Direct Push Allowed? |
|---|---|---|---|:---:|
| `main` | Production releases. Always stable and tagged (`v*`). | — | — | ❌ Never |
| `develop` | Integration branch for next release. Contains latest tested code. | `main` | `main` (via release) | ❌ Never |
| `feature/*` | New features and enhancements. | `develop` | `develop` | ✅ Branch owner |
| `bugfix/*` | Non-critical bug fixes for development code. | `develop` | `develop` | ✅ Branch owner |
| `release/*` | Release stabilization, version bumps, final validation. | `develop` | `main` **and** `develop` | ❌ PR only |
| `hotfix/*` | Urgent production fixes directly on released code. | `main` | `main` **and** `develop` | ❌ PR only |

---

## Branch Naming Conventions

All branch names must be lowercase, use kebab-case, and describe the intent clearly:

```text
<type>/<optional-ticket-or-issue-id>-<short-description>
```

### Valid Prefixes

| Prefix | Description | Example |
|---|---|---|
| `feature/` | New user-facing feature or CLI capability | `feature/ABC-101-interactive-prompt`<br>`feature/add-dockerfile-template` |
| `bugfix/` | Fix for a non-urgent bug in `develop` | `bugfix/ABC-204-fix-hardlink-error`<br>`bugfix/tui-cursor-glitch` |
| `hotfix/` | Urgent fix for a bug found in production (`main`) | `hotfix/ABC-999-fix-critical-crash`<br>`hotfix/v1.0.1-exit-code-panic` |
| `release/` | Preparing a new production release | `release/v1.1.0` |
| `refactor/` | Code refactoring without behavioral changes | `refactor/standardize-project`<br>`refactor/tui-state-cleanup` |
| `docs/` | Pure documentation updates | `docs/contributing-workflow` |
| `test/` | Adding or updating unit/integration tests | `test/add-scaffold-concurrency-tests` |

> [!NOTE]
> If working on a ticket (e.g., Jira `AIAI-123` or GitHub Issue `#45`), include the identifier in the branch name:
> `feature/AIAI-123-custom-template-vars` or `bugfix/gh-45-windows-path-separator`

---

## Commit Message Conventions

We adhere strictly to [Conventional Commits](https://www.conventionalcommits.org/):

```text
<type>(<scope>): <subject>

[optional body explaining WHY, not WHAT]

[optional footer(s), e.g. Closes #123]
```

### Types

- `feat`: A new feature
- `fix`: A bug fix
- `refactor`: Code change that neither fixes a bug nor adds a feature
- `perf`: Performance improvement
- `test`: Adding missing tests or correcting existing tests
- `docs`: Documentation-only changes
- `style`: Changes that do not affect the meaning of code (formatting, whitespace)
- `chore`: Build tasks, configuration changes, dependency bumps
- `ci`: Changes to CI/CD workflows and configuration

### AIAI CLI Scopes

Recommended scopes for this codebase:

- `tui`: Bubble Tea terminal interface, models, styles, views
- `command`: Cobra commands, flags, arguments parsing
- `template`: Embedded templates, manifest parsers, variables
- `scaffold`: Plan/apply engine, file executor, hash verification
- `platform`: Filesystem containment, terminal detector
- `deps`: Dependencies update in `go.mod`
- `release`: GoReleaser, installation scripts, release assets

### Examples

```text
feat(tui): enhance terminal UI with command palette and improved navigation
fix(scaffold): resolve hardlink publication failure on external filesystems
refactor(template): strictly validate manifest field names on load
docs(readme): add troubleshooting section for Windows terminals
test(command): add golden test for json dry-run output
```

---

## Development Workflow (Step-by-Step)

### 1. Prerequisites & Environment Setup

Ensure you have the following installed locally:
- **Go**: 1.27+ (`go version`)
- **Git**: 2.30+
- **golangci-lint**: latest v2 (`golangci-lint --version`)
- **gofumpt**: v0.9.2 (`go install mvdan.cc/gofumpt@v0.9.2`)
- **Node.js**: 22+ & **Corepack** (for generated Python template verification)
- **uv**: Astral's package manager for Python testing

Clone the repository and verify the build:

```bash
git clone https://github.com/AIAI-Laboratory/aiai-cli.git
cd aiai-cli

# Verify build
go build -trimpath -o bin/aiai ./cmd/aiai
./bin/aiai version
```

### 2. Starting a New Task

Always start by syncing your local `develop` branch with the upstream remote:

```bash
# Checkout and sync develop
git checkout develop
git pull origin develop

# Create your feature or bugfix branch
git checkout -b feature/AIAI-101-interactive-prompt
```

### 3. Local Development & Verification

Make your changes following the existing architecture:
- `cmd/aiai`: Entrypoint and CLI flags
- `internal/app`: Dependency wiring
- `internal/command`: Cobra commands (`init`, `version`)
- `internal/tui`: Bubble Tea v2 terminal interface
- `internal/scaffold`: Plan and apply engine
- `internal/template`: Manifest validation and embedding

#### Quality Verification Commands

Before committing, run the project verification suite:

```bash
# 1. Format code with gofumpt
go run mvdan.cc/gofumpt@v0.9.2 -w .

# 2. Run Go vet
go vet ./...

# 3. Run full test suite with race detector
go test -race -v ./...

# 4. Run linter
golangci-lint run

# 5. Check for known vulnerabilities
go run golang.org/x/vuln/cmd/govulncheck@latest ./...

# 6. Validate GoReleaser config
goreleaser check
```

#### Template Verification

If you modified any templates in `templates/`:

```bash
# Generate a test project in a temporary location
go run ./cmd/aiai init python smoke-test --target /tmp/smoke-test --yes

# Verify generated project
cd /tmp/smoke-test
git init
corepack enable
pnpm install
uv lock
uv sync --locked
uv run --locked pytest
```

### 4. Committing Changes

Stage your files and write a conventional commit message:

```bash
git add internal/tui/ internal/command/
git commit -m "feat(tui): add fuzzy filtering to command palette"
```

### 5. Keeping Your Branch Updated

Before opening a Pull Request, rebase your branch on the latest `develop`:

```bash
git fetch origin develop
git rebase origin/develop

# If conflicts occur, resolve them, then:
# git add <resolved-files>
# git rebase --continue
```

### 6. Opening a Pull Request

Push your branch to GitHub:

```bash
git push -u origin feature/AIAI-101-interactive-prompt
```

Then create a Pull Request on GitHub:
- **Base repository**: `AIAI-Laboratory/aiai-cli`
- **Base branch**:
  - `develop` for `feature/*`, `bugfix/*`, `refactor/*`, `test/*`, `docs/*`
  - `main` **only** for `release/*` and `hotfix/*`
- **Title**: Follow Conventional Commits format (e.g. `feat(tui): add fuzzy filtering to command palette`)
- **Description**: Fill out the Pull Request template completely.

### 7. Code Review & Merge

1. **Automated CI**: GitHub Actions runs the test matrix (Linux, macOS, Windows), linter, vulnerability check, and generated project verification.
2. **Reviewers**: At least one maintainer must review and approve the PR.
3. **Merging**:
   - Features & bugfixes are typically merged using **Squash and Merge** (or **Rebase and Merge**) into `develop`.
   - Delete your branch after successful merge.

---

## Release Process

When `develop` has accumulated sufficient features and is ready for a production release:

```text
develop ───► release/v1.2.0 ───► [Test & Polish] ───► PR into main (Tag v1.2.0)
                                                 └───► PR back into develop
```

1. Create a release branch from `develop`:
   ```bash
   git checkout develop
   git pull origin develop
   git checkout -b release/v1.2.0
   ```
2. Finalize version numbers, documentation, and changelog.
3. Run the full verification test suite.
4. Open a PR from `release/v1.2.0` into `main`.
5. Once merged into `main`, tag the release:
   ```bash
   git checkout main
   git pull origin main
   git tag -a v1.2.0 -m "Release v1.2.0"
   git push origin v1.2.0
   ```
   > Pushing tag `v*` automatically triggers `.github/workflows/release.yml` which builds and publishes multi-platform binaries via GoReleaser.
6. Merge `release/v1.2.0` back into `develop` to sync any release-specific fixes.
7. Delete `release/v1.2.0`.

---

## Hotfix Process (Emergency Fixes)

If a critical bug is discovered in production (`main`):

```text
main (v1.2.0) ───► hotfix/fix-panic ───► [Fix & Test] ───► PR into main (Tag v1.2.1)
                                                      └───► PR into develop
```

1. Branch immediately from `main`:
   ```bash
   git checkout main
   git pull origin main
   git checkout -b hotfix/ABC-999-fix-panic
   ```
2. Implement the minimal fix and verify with tests.
3. Open a PR targeting `main`.
4. After review and merge into `main`, tag a patch release (e.g., `v1.2.1`).
5. Backport the fix into `develop`:
   ```bash
   git checkout develop
   git pull origin develop
   git merge hotfix/ABC-999-fix-panic # or cherry-pick the commit
   git push origin develop
   ```
6. Delete the hotfix branch.

---

## Pull Request Checklist

Before submitting your PR, verify:

- [ ] Target branch is set correctly (`develop` for features/bugfixes, `main` for hotfix/release).
- [ ] Code follows project conventions and passes `go run mvdan.cc/gofumpt@v0.9.2 -w .`.
- [ ] Linter passes with zero warnings (`golangci-lint run`).
- [ ] All unit and integration tests pass (`go test -race ./...`).
- [ ] Any modified templates have valid manifests and pass verification.
- [ ] Commit history is clean and conforms to Conventional Commits.
- [ ] Relevant documentation has been updated.
