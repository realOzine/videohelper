# Repository Guidelines

## Project Structure & Module Organization

VideoHelper combines a Go backend (`cmd/videohelper/`, `internal/`) with a Python model service (`ml/app/`). Tests live beside Go packages and in `ml/tests/`. Database migrations are in `migrations/`; configuration and the shared ML contract are in `config/` and `contracts/`. Keep the Chinese architecture documents, `docs/design.md` and `docs/spec.md`, aligned with contract changes. Runtime media belongs in object storage; local temporary files use `.local/`.

## Agent Scope: Ignore `model-test/`

The entire `model-test/` directory is an experimental sandbox for validation, demos, and scratch work. Unless the user explicitly requests work there, do not read, list, search, execute, modify, move, delete, or otherwise operate on it or its contents. Exclude it from searches, builds, tests, and refactors. General repository work does not authorize access.

## Build, Test, and Development Commands

- `make deps-up` / `make deps-down`: start/stop local Postgres, Redis, and MinIO; preserve volumes.
- `make build`: build `bin/videohelper`; see `README.md` for local processing and search.
- `make test-go` / `make vet`: scoped Go race tests and static checks.
- `make fmt` / `make generate`: format Go and regenerate package-local sqlc outputs.
- `cd ml && uv run pytest` / `uv run ruff check .`: Python tests and lint.

## Coding Style & Testing Guidelines

Use `gofmt`, Python four-space indentation, and Ruff. Keep functions focused and reuse existing helpers. SQL uses singular snake_case names; wire fields use snake_case, IDs are strings, and intervals are half-open integer milliseconds. Do not hand-edit `internal/*/*db/` generated code.

Name Go tests `Test...` in `_test.go`, and Python tests `test_...`. Add meaningful checks for non-trivial behavior. Integration tests require explicit `VH_TEST_*` variables and disposable databases. No numerical coverage target is set. Fake-model tests do not establish real-model retrieval quality.

## Commit & Pull Request Guidelines

History uses short imperative messages, sometimes `feat:` or `test:` prefixes. Commit small verified units. PRs should explain behavior, scope, relevant issues, and checks actually run.

Use isolated worktrees for parallel execution with `gpt-6.1-sol` at high effort. The coordinator reviews, merges into `main`, then removes merged worktrees. Avoid development journals. Ponytail full applies: minimal complete changes, preserving validation, cancellation, and data safety.

## Current Implementation Boundary

Local text search uses WeMM-Embedding-2B for video, text, and image encoding, ASR for `speech`, and CLAP HTSAT unfused for the `audio` sound channel. Preserve source audio for ASR and CLAP. Pass query text unchanged; English translation is deferred. Frontend, LLM/Agent, and AWS acceptance remain deferred.
