## Description

Provide a brief summary of the changes introduced in this PR and why they are needed.

Fixes / Closes #(issue)

## Type of Change

- [ ] `feat`: A new feature
- [ ] `fix`: A bug fix
- [ ] `refactor`: Code change that neither fixes a bug nor adds a feature
- [ ] `perf`: Performance improvement
- [ ] `docs`: Documentation only changes
- [ ] `test`: Adding or correcting tests
- [ ] `chore`: Maintenance, dependencies, build tasks
- [ ] `ci`: CI/CD workflow changes

## Target Branch

- [ ] `develop` (for `feature/*`, `bugfix/*`, `refactor/*`, `docs/*`)
- [ ] `main` (for `hotfix/*`, `release/*` only)

## Scope

- [ ] `tui`: Terminal UI and Bubble Tea components
- [ ] `command`: Cobra commands and CLI flags
- [ ] `template`: Embedded templates or manifest parsing
- [ ] `scaffold`: Plan and apply engine, file execution
- [ ] `platform`: Filesystem containment or terminal detection
- [ ] other:

## How Has This Been Tested?

Describe the tests run to verify the changes:
- [ ] Unit tests: `go test -race ./...`
- [ ] Formatting: `make fmt-check` (or `./scripts/fmt.sh`)
- [ ] Linting: `golangci-lint run`
- [ ] Vulnerability audit: `go run golang.org/x/vuln/cmd/govulncheck@latest ./...`
- [ ] Template verification (if template modified): `go run ./cmd/aiai init ...`

## Checklist

- [ ] My code follows the style guidelines of this project.
- [ ] I have self-reviewed my code.
- [ ] I have commented my code where necessary, particularly in hard-to-understand areas.
- [ ] I have made corresponding changes to the documentation.
- [ ] My commit messages follow the [Conventional Commits](https://www.conventionalcommits.org/) format.
