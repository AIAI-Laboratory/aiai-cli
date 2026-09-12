# AIAI CLI

A deterministic project scaffolder with a Cobra command interface and a Bubble
Tea terminal UI. Templates are compiled into the binary; generation works offline
and does not run subprocesses or install dependencies.

## Build and run

Go 1.27+ is required to build from source. Released binaries require no Go runtime.

```bash
go build -trimpath -o bin/aiai ./cmd/aiai
./bin/aiai
```

The home menu provides **Project → Initialize Python Project**, keyboard help,
and version information. The wizard selects a template, collects project fields,
previews file operations, and asks for confirmation. Use arrows or `j/k` in menus,
Tab in forms, Enter to select, Esc to return, and Ctrl+C to cancel. `?` opens help.

```bash
aiai init python my-project --dry-run
aiai init python my-project --yes
aiai init python . --name my-project --package my_project --yes
aiai init python my-project --target /path/to/project --yes --json
aiai version
```

`aiai` and `aiai init` open the TUI only with a terminal. Non-interactive generation
requires `--yes`; `--dry-run` never writes and does not require confirmation.
In a terminal, an initialization command always displays a preview and asks for
confirmation even with `--yes`. A dry run displays the preview and exits.

Positional names are normalized (`My_Project` → `my-project`) for the directory
and distribution name; the default Python package is `my_project`. `.` uses the
current directory. `--target` overrides the destination; `--name` overrides project
metadata; `--package` overrides the import name. With only `--name`, the normalized
name becomes the destination. Explicit target paths may be absolute.

## Python template

`python-minimal` v1 generates a root Python package, a smoke test, Python 3.13
configuration, uv dependencies, Ruff, Pyright, Pytest, Husky with pnpm, GitHub
Actions, and Dependabot for uv and Actions. It is a minimal application layout
without a publishing build backend.

After generation, run the printed setup commands inside the destination:

```bash
git init
corepack enable
pnpm install
uv lock
uv sync
```

Install Git, uv, Node.js 22+ and Corepack first. `pnpm install` activates Husky via
the root `prepare` script. Commit the resulting `uv.lock` and `pnpm-lock.yaml`
before pushing: generated CI requires a lockfile and uses `uv sync --locked`.
Husky checks formatting, lint, types, and tests through uv. pnpm and Husky updates
are manual; Dependabot intentionally monitors only uv and GitHub Actions.

## Filesystem behavior

Generation follows **Plan → Apply**. The entire template is rendered and validated
before writes. Unrelated files are preserved; identical files are skipped.
Conflicting regular files require `--force`; directories and symlinks at template
destinations are refused. Never use `--force` without reviewing the preview.

The executor revalidates file hashes before applying a plan and before each write.
Go's rooted filesystem APIs constrain writes to the target. New files use an
atomic hard-link publication to avoid clobbering a file that appears concurrently;
overwrites use a temporary sibling and rename. Filesystems must support hard links
(for example APFS, ext4, NTFS); unsupported filesystems fail with an execution error.
The generator does not lock against unrelated programs editing the same directory.

Generation is atomic per file, not per project. On failure or cancellation, completed
files remain and are reported; temporary files are removed. Empty directories may
remain. Rerun to skip already-generated files and finish. Overwritten content has
no automatic backup. `--force` applies only to files listed by the template.

## Output contract

`--json` emits one object with `schema_version: 1`, `status`, and optional `plan`,
`result`, `error`, and `version` fields. A plan contains destination, template ID
and version, warnings, and sorted operations (`path`, `action`, numeric `mode`).
File bodies and internal hashes are excluded. Results report completed and skipped
paths plus setup commands. Errors include a numeric exit code and message, with
partial results when generation stopped. Human prompts and verbose logs use stderr.
`--no-color` and `NO_COLOR` disable TUI colors.

| Exit | Meaning |
| --- | --- |
| 0 | Success or conflict-free dry run |
| 2 | Usage or validation error |
| 3 | File conflict or stale preview |
| 4 | Filesystem or terminal execution failure |
| 130 | Cancellation |

## Architecture and development

`cmd/aiai` delegates to `internal/app`, which wires dependencies explicitly.
`internal/command` and `internal/tui` share `project.Initializer` and the scaffold
executor. The project layer validates names; the scaffold layer renders, plans,
and applies files. `internal/template` reads strict manifests through a registry;
`templates/embed.go` embeds the assets including dotfiles and Python underscored
modules. `internal/platform` owns terminal detection and filesystem containment.
All service interfaces are internal; there is no public Go SDK.

Add a template under `templates/`, declare every file and scalar variable in its
manifest, register its ID in `internal/template/embedded.go`, then add reviewed
golden fixtures and integration tests. Template versions change with AIAI releases.
Templates receive only project/package strings and have no shell, network, or
arbitrary filesystem helpers. YAML uses `go.yaml.in/yaml/v3` with unknown-field
rejection. Bubble Tea, Bubbles, and Lip Gloss use their v2 APIs.

```bash
go test -race ./...
go vet ./...
go run mvdan.cc/gofumpt@v0.9.2 -w .
golangci-lint run
govulncheck ./...
goreleaser check
```

CI tests on Linux, macOS, and Windows and verifies a generated Python project and
Husky installation. GoReleaser packages amd64/arm64 binaries and checksums for all
three platforms. Pushing a `v*` tag triggers the GitHub release workflow.

MIT licensed. No AI provider, network template loading, or external plugins in v1.
