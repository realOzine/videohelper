# Repository Guidelines

## Project Structure & Module Organization

VideoHelper is a video-search backend under implementation. `docs/design.md` describes product and architecture decisions; `docs/spec.md` defines implementation contracts. Go packages live in `internal/`, migrations in `migrations/`, and service configuration in `config/`. Preserve their Chinese prose and distinguish confirmed decisions from proposals and open questions.

## Agent Scope: Ignore `model-test/`

The entire `model-test/` directory is an experimental sandbox for validation, demos, and scratch work. Ignore it by default. Unless the user explicitly requests work within this directory, do not read, list, search, execute, modify, move, delete, or otherwise operate on it or its contents. Exclude it from repository-wide inspection, searches, builds, tests, and refactors. A general repository task does not authorize access to this directory.

## Implementation Workflow

Use the assigned isolated Git worktree. Execute small tasks with `gpt-6.1-sol` at high reasoning effort when delegating. Commit each verified unit; the coordinating agent reviews and merges it into `main`, then removes the merged worktree. Do not write development diaries or acceptance-report documents. Keep `docs/design.md` and `docs/spec.md` aligned when contracts change; the coordinator owns shared-contract decisions. Frontend `web/` is deferred.

Ponytail full applies: reuse existing code and standard-library features, keep each function focused, and avoid speculative abstractions or redundant internal validation. Preserve trust-boundary validation, error handling, cancellation, and data safety. Add a small meaningful test for non-trivial logic, and distinguish module tests, integration tests, and real-model verification.

## Current Implementation Boundary

Build local core search first. Use WeMM-Embedding-2B for video, text, and image encoding; search input is text only. Keep ASR and the `speech` channel alongside `visual`. Do not enable the `audio` sound-retrieval channel or CLAP. Frontend, LLM/Agent integration, and AWS deployment/acceptance are deferred. Preserve source audio for ASR. Explicit fake tests do not establish real-model retrieval quality.
