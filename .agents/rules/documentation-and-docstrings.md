---
trigger: always_on
description: Standards for writing documentation, Go doc comments, and Python docstrings when adding or modifying functions.
---

# Documentation & Docstring Rules

When adding or modifying functions, methods, types, or modules in this repository, follow these documentation standards:

## 1. Go Code (`cmd/`, `internal/`)
- **Exported symbols**: Every exported function, method, struct, interface, constant, and variable **must** have a Go doc comment immediately above its declaration, starting with the symbol's exact name (e.g., `// Plan builds a deterministic scaffold plan for the target directory.`).
- **Unexported helpers**: Add a concise doc comment for non-trivial unexported functions or helpers explaining their purpose, invariants, and edge cases.
- **Error & concurrency semantics**: Document non-obvious return values, sentinel errors (`ErrValidation`, `ErrConflict`, etc.), side effects, and thread-safety guarantees when relevant.
- **Test helpers**: Keep test helper comments brief; document what setup/assertion invariant they enforce.

## 2. Python Templates (`templates/`)
- Every function, method, and class in Python template files **must** include a PEP 257 triple-quoted docstring (`"""..."""`) summarizing its purpose, parameters, return value, and raised exceptions when non-obvious.

## 3. Inline Comments
- Comment **why** a non-obvious decision, workaround, or ordering constraint exists—not **what** the next line literally does.
- Preserve existing unrelated docstrings and comments when editing files.

## 4. Keep Docs in Sync
- When changing CLI commands, flags, or user-facing workflows, update the corresponding Cobra `Short`/`Long` descriptions and relevant documentation under `docs/` or `README.md`.
- When bumping the `go` toolchain version in `go.mod` (for example to fix a `govulncheck` advisory), update the matching minimum Go requirement in `README.md` and `docs/CONTRIBUTING.md`.
