# Repository Guidelines

## Desktop Scope and Structure

VideoHelper is a personal cross-platform desktop app, with macOS Apple Silicon first. `cmd/videohelper-desktop/` owns startup/lifecycle; `desktop/` hosts MyGo; `web/` contains the embedded React UI. `internal/desktop/` owns SQLite/sqlite-vec, local files, durable jobs, local UI endpoints and private runtime setup. Reuse `internal/media/`, `internal/mlclient/`, `internal/retrieval/` and `internal/timeline/`. Python models remain in `ml/app/`, `ml/plugins/`, and locked `ml/profiles/`. Keep Chinese `docs/design.md`, `docs/spec.md` and the ML contract aligned.

There are no accounts, tenants, JWT, public admin APIs, per-user limits, billing, PostgreSQL, Redis, S3 or Docker dependencies. Keep the desktop focused on responsiveness, clear UI, compatibility and search. The short-lived loopback capability and managed-file boundaries protect local process communication; they are not a user login system.

## Build and Validation

- `npm --prefix web ci` installs frontend development dependencies.
- `make desktop` / `make build` builds the embedded desktop executable; `make desktop-run` starts MyGo development mode.
- `make test-go`, `make test-web`, `make vet` run scoped checks; do not use an unrestricted repository-wide test command.
- Python checks use the managed development environment in `ml/`: `uv run pytest` and `uv run ruff check .`.
- Real tool/model tests require explicit `VH_TEST_DESKTOP_*` directories. Use disposable task-owned data and preserve useful evidence. Fake-model tests do not establish real retrieval quality.
- Runtime media belongs in the selected application data directory. Repository test artifacts belong in `.local/`; evaluation evidence follows `Eval/AGENTS.md`.

## Implementation Rules

Use gofmt, Python four-space indentation and Ruff. Keep functions focused, reuse existing helpers, add meaningful tests for non-trivial logic. SQL tables use singular snake_case, wire IDs are strings, and time intervals are half-open integer milliseconds.

Preserve original media for ASR and CLAP. Pass query text unchanged. Keep model fingerprints and backend indexes isolated. LLM/Agent remains deferred. A single distributable executable may release components and download models; do not require the user's global Python, FFmpeg, yt-dlp or external services.

## Agent Workflow

- Start implementation in an isolated feature worktree; keep `main` for reviewed changes. Preserve unrelated changes and user data.
- Enable Ponytail FULL for every execution, including sub-agents: minimal complete changes with validation, cancellation and data safety intact.
- Use `gpt-6.1-sol` at high effort for implementation sub-agents. Independent parallel tasks use separate branches/worktrees based on the feature branch.
- Work in small verified units, commit frequently, and integrate checked units into the feature branch promptly.
- Update affected documentation without development journals or redundant acceptance reports.
- Review and verify the combined feature before merging into `main`. Clean merged task worktrees/branches and task-created temporary services after preserving useful validation evidence. Keep the requested feature worktree while work remains there.
