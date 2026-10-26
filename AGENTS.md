# Repository Guidelines

## Project Structure & Module Organization

VideoHelper combines a Go backend (`cmd/videohelper/`, `internal/`) with a Python model service (`ml/app/`) and a React frontend (`web/`). Tests live beside Go packages and in `ml/tests/`. Database migrations are in `migrations/`; configuration and the shared ML contract are in `config/` and `contracts/`. Keep the Chinese architecture documents, `docs/design.md` and `docs/spec.md`, aligned with contract changes. Runtime media belongs in object storage; local temporary files use `.local/`.

## Agent Scope: Ignore `model-test/`

The entire `model-test/` directory is an experimental sandbox for validation, demos, and scratch work. Unless the user explicitly requests work there, do not read, list, search, execute, modify, move, delete, or otherwise operate on it or its contents. Exclude it from searches, builds, tests, and refactors. General repository work does not authorize access.

## Build, Test, and Development Commands

- `make deps-up` / `make deps-down`: start/stop local Postgres, Redis, and MinIO; preserve volumes.
- `make build`: build `bin/videohelper`; see `README.md` for local processing and search.
- `make test-go` / `make vet`: scoped Go race tests and static checks.
- `make fmt` / `make generate`: format Go and regenerate package-local sqlc outputs.
- `cd ml && uv run pytest` / `uv run ruff check .`: Python tests and lint.
- `npm --prefix web ci` / `npm --prefix web run dev` / `npm --prefix web run build`: install, serve on 5173, or type-check and build `web/dist`.
- `make test-web`: frontend helper tests (Node 22+).

## Coding Style & Testing Guidelines

Use `gofmt`, Python four-space indentation, and Ruff. Keep functions focused and reuse existing helpers. SQL uses singular snake_case names; wire fields use snake_case, IDs are strings, and intervals are half-open integer milliseconds. Do not hand-edit `internal/*/*db/` generated code.

Name Go tests `Test...` in `_test.go`, and Python tests `test_...`. Add meaningful checks for non-trivial behavior. Integration tests require explicit `VH_TEST_*` variables and disposable databases. No numerical coverage target is set. Fake-model tests do not establish real-model retrieval quality.

## Commit & Pull Request Guidelines

History uses short imperative messages, sometimes `feat:` or `test:` prefixes. Commit small verified units. PRs should explain behavior, scope, relevant issues, and checks actually run.

## Agent Development Workflow

- Start each new implementation on a new feature branch in a new, isolated worktree. Keep `main` for reviewed changes.
- Enable Ponytail FULL for every execution, including sub-agents: make minimal complete changes while preserving validation, cancellation, and data safety.
- Use `gpt-6.1-sol` at high effort for implementation sub-agents. Give independent parallel tasks separate branches and worktrees based on the feature branch.
- Work in small, verifiable units and commit frequently. Merge each completed and checked unit into the feature branch promptly.
- Update affected documentation, especially `docs/design.md` and `docs/spec.md`, alongside implementation. Avoid development journals and redundant acceptance reports.
- Review and verify the combined feature before merging it into `main`. Then remove the worktrees, merged task branches, and temporary services or artifacts created by this work; preserve unrelated changes, user data, and useful validation evidence.

## Current Implementation Boundary

Local text search uses WeMM-Embedding-2B for video, text, and image encoding, ASR for `speech`, and CLAP HTSAT unfused for the `audio` sound channel. Preserve source audio for ASR and CLAP. Pass query text unchanged; English translation is deferred. The `web/` frontend is React, TypeScript and Tailwind on Vite, one HTML entry per page; style with the theme tokens in `web/src/index.css`. LLM/Agent and AWS acceptance remain deferred.
