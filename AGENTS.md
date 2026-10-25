# Repository Guidelines

## Project Structure & Module Organization

VideoHelper is currently a design-stage video-search project. Tracked content consists of `docs/design.md` (product, architecture, decisions) and `docs/spec.md` (implementation contracts and testing strategy). Preserve their Chinese prose and distinguish confirmed decisions from proposals and open questions.

## Agent Scope: Ignore `model-test/`

The entire `model-test/` directory is an experimental sandbox for validation, demos, and scratch work. Ignore it by default. Unless the user explicitly requests work within this directory, do not read, list, search, execute, modify, move, delete, or otherwise operate on it or its contents. Exclude it from repository-wide inspection, searches, builds, tests, and refactors. A general repository task does not authorize access to this directory.

## Implementation Workflow

Use the assigned isolated Git worktree. Execute small tasks with `gpt-6.1-sol` at high reasoning effort when delegating. Commit each verified unit; the coordinating agent reviews and merges it into `main`, then removes the merged worktree. Do not write development diaries or acceptance-report documents. Keep `docs/design.md` and `docs/spec.md` aligned when contracts change; the coordinator owns shared-contract decisions. Frontend `web/` is deferred.

Ponytail full applies: reuse existing code and standard-library features, keep each function focused, and avoid speculative abstractions or redundant internal validation. Preserve trust-boundary validation, error handling, cancellation, and data safety. Add a small meaningful test for non-trivial logic, and distinguish module tests, integration tests, and real-model verification.
