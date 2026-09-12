# Contributing to AIAI CLI

We welcome contributions from everyone!

Please refer to our comprehensive contribution and Git Flow workflow documentation:

- 📖 **[Contribution Guide & Git Flow Workflow](docs/CONTRIBUTING.md)**

## Quick Reference

- **Default base branch for PRs**: `develop`
- **Branch naming**: `feature/<id>-<desc>`, `bugfix/<id>-<desc>`, `hotfix/<desc>`
- **Commit style**: [Conventional Commits](https://www.conventionalcommits.org/) (`feat:`, `fix:`, `refactor:`, etc.)
- **Format & Lint**:
  ```bash
  go run mvdan.cc/gofumpt@v0.9.2 -w .
  golangci-lint run
  go test -race ./...
  ```
