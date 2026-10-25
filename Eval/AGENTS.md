# Eval workspace

- Keep new evaluation tools, datasets, source assets, generated media, annotations, reports and future evaluation runners inside this `Eval/` directory.
- Keep existing application tests in their current locations; this directory does not authorize moving them.
- Do not access `../model-test/`.
- Distinguish synthetic script-derived labels, assistant visual review and independent human ground truth. Never report retrieval quality from corpus validation alone.
- Preserve frozen corpus bytes and hashes. Create a new dataset version when changing media or semantic labels.

- Integrate evaluation development into branch `eval`, not `main`. Commit small verified units. Parallel agents use independent worktrees with `gpt-6.1-sol` at high effort; review and merge into `eval`, then remove merged worktrees.
- `checksums.sha256` freezes corpus assets, annotations and videos only. Tools and reports can evolve without changing corpus identity.
