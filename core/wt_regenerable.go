package main

// WT_REGENERABLE / WT_ARCHIVABLE (bin/hw:9122-9133): the ignored paths a worktree may lose without losing work. ONE list, read by
// `hw done` (t4_done_wt.go), `hw reap` (reap.go) and `hw reap --apply` (t4_reap_wt.go): the three used to carry a copy each, and
// a name added to one left the other two flagging it «DO NOT REMOVE» (gpui-kit.d.ts and jsconfig.json, regenerable and listed in
// herdr-app/.gitignore, 2026-11-11). A path is disposable when any of its components is a name here.
const wtRegenerable = ".agents .atl .claude .codex .opencode .hw .specify .venv .ruff_cache .pytest_cache .mypy_cache __pycache__ node_modules dist build .next .turbo .DS_Store AGENTS.md PROGRESS.md opencode.json local next-env.d.ts tsconfig.tsbuildinfo tsconfig.build.tsbuildinfo .astro gpui-kit.d.ts jsconfig.json"
const wtArchivable = ".artifacts qa-report test-results playwright-report"
