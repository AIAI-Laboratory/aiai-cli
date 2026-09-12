# Contributing to AIAI CLI

We welcome contributions from everyone!

Please refer to our comprehensive contribution and Git Flow workflow documentation:

- 📖 **[Contribution Guide & Git Flow Workflow](docs/CONTRIBUTING.md)**

## Quick Reference

- **Setup Git hooks**: `make setup` (or `npm install`)
- **Default base branch for PRs**: `develop`
- **Branch naming**: `feature/<id>-<desc>`, `bugfix/<id>-<desc>`, `hotfix/<desc>`
- **Commit style**: [Conventional Commits](https://www.conventionalcommits.org/) (`feat:`, `fix:`, `refactor:`, etc.)
- **Format & Lint**:
  ```bash
  make fmt            # or ./scripts/fmt.sh (runs goimports and gofumpt)
  make lint           # runs golangci-lint
  go test -race ./...
  ```
