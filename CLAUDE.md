# Project Instructions for AI Agents

This file provides instructions and context for AI coding agents working on this project.

## Communication

Adhere to the ASD-STE100 standard for our communication, as well as for writing tasks, comments and git commits. Spelling can be a mix og American and British english. ADRs are exempt from this requirement.

If anything in a task or my instructions is ambigious or under-specifed, ask for clarifications.

## Architecture

When working on generic functionality or utilities, consider whether it's possible to solve by adding a well-tested and proven dependency.

If my instructions are vague, ambiguous or are over-looking something, feel free to doubt and press for more reflection or answers.

## Go style

When structuring a package, prefer to place constant, variable and type declarations at the top. Functions and member functions should come after that.

When creating a test, prefer keeping the test functions at the top of the file and the helper functions below.

If you cannot keep the comments brief and succint, refrain from commenting the code. A comment MUST NOT exceed 3 lines. If absolutely necessary, go beyond the ASD-STE100 standard.

Comment functions for their purpose, structs for the conceptual meaning, variables for the state they hold and test functions for what case they exercise.

Variable comments must be placed after the variable declaration with //. If the purpose of a variable is obvious, eg. when used for interface checks, comments should be omitted.

## Git

Do not commit until explicity instructed to do so.

Attempt to bundle related changes into the same commit. It is acceptable to create multiple commits in order to maintain this order. Keep commit messages brief. They MUST NOT excced 5 lines.

At handoff, report changed files, validation performed, and suggested next commands or beads task.

If you are a subagent working on a task that makes code changes, create a worktree like this `git worktree add ./worktrees/son-XXX -b branch-name`. The directory should be named for the beads task that is being worked on, while the branch should be named for the contents. This is the one exception to the rule above: A subagent commits in its own worktree without being asked. Do not ever merge to main repository.

## Build & Test

Prefer Makefile targets over raw command-line invocations for building and testing (e.g. `make test` over `go test ./...`). Only fall back to the command line when no target covers what's needed.

Current targets: `build`, `fmt`, `vet`, `test`, `test-verbose`, `test-race`, `test-live`, `run`, `run-nocache`, `tidy`, `adr-html`, `help` — see `Makefile`.

`adr-html` renders each `ADRs/*.md` to a full standalone HTML page (`ADRs/html/<adr-basename>/index.html`) with diagrams rendered as linked SVGs (via `mmdc` + `npx marked`, not inlined, no Docker, no publishing).

`test-live` runs tests gated behind the `-live` flag, which require a real network with real Sonos speakers. The default `test`/`test-verbose`/`test-race` targets never touch the network.

## Defining tasks

Always ask me if there's any ambiguity to the instructions or tasks I've given. It's more important to get things correct than being fast.

## Task hierarchy

Use a parent for containment. Use a dependency for order.

A parent says "this is part of that epic". A dependency says "this must wait for that". Do not use a dependency to record that two tasks belong to one piece of work.

Start an epic when new work splits into several tasks. Do not make an epic out of tasks that already exist: a reparented task keeps its old id, so its id and its place in the tree disagree from then on.

## Task labels

`diagnose-first` marks a task where a plausible answer can be wrong and still look right. Measure before building, and read the task before dispatching it: it wants a model that can hold competing hypotheses, and its cheap fix will appear to work.

Label the property, not the model. Model names change; a task outlives them, and the reason is what a person needs anyway.

<!-- BEGIN BEADS INTEGRATION v:1 profile:minimal hash:6cd5cc61 -->
## Beads Issue Tracker

This project uses **bd (beads)** for issue tracking. Run `bd prime` to see full workflow context and commands.

bd is initialized in stealth mode, so is local to this repo and is not available to anyone else. Do not reference beads tasks in ADRs, comments or commit messages.

### Quick Reference

```bash
bd ready              # Find available work
bd show <id>          # View issue details
bd update <id> --claim  # Claim work
bd close <id>         # Complete work
```

### Rules

- Use `bd` for ALL task tracking — do NOT use TodoWrite, TaskCreate, or markdown TODO lists
- Run `bd prime` for detailed command reference and session close protocol
- Use `bd remember` for persistent knowledge — do NOT use MEMORY.md files

**Architecture in one line:** issues live in a local Dolt DB; sync uses `refs/dolt/data` on your git remote; `.beads/issues.jsonl` is a passive export. See https://github.com/gastownhall/beads/blob/main/docs/SYNC_CONCEPTS.md for details and anti-patterns.

## Agent Context Profiles

The managed Beads block is task-tracking guidance, not permission to override repository, user, or orchestrator instructions.

- **Conservative (default)**: Use `bd` for task tracking. Do not run git commits, git pushes, or Dolt remote sync unless explicitly asked. At handoff, report changed files, validation, and suggested next commands.
- **Minimal**: Keep tool instruction files as pointers to `bd prime`; use the same conservative git policy unless active instructions say otherwise.
- **Team-maintainer**: Only when the repository explicitly opts in, agents may close beads, run quality gates, commit, and push as part of session close. A current "do not commit" or "do not push" instruction still wins.

## Session Completion

This protocol applies when ending a Beads implementation workflow. It is subordinate to explicit user, repository, and orchestrator instructions.

1. **File issues for remaining work** - Create beads for anything that needs follow-up
2. **Run quality gates** (if code changed) - Tests, linters, builds
3. **Update issue status** - Close finished work, update in-progress items
4. **Hand off** - Summarize changes, validation, issue status, and any blocked sync/commit/push step

**Critical rules:**
- Explicit user or orchestrator instructions override this Beads block.
- Do not commit or push without clear authority from the active profile or the current user request.
- If a required sync or push is blocked, stop and report the exact command and error.
<!-- END BEADS INTEGRATION -->

<!-- rtk-instructions v2 -->
# RTK (Rust Token Killer) - Token-Optimized Commands

## Golden Rule

**Always prefix commands with `rtk`**. If RTK has a dedicated filter, it uses it. If not, it passes through unchanged. This means RTK is always safe to use.

**Important**: Even in command chains with `&&`, use `rtk`:
```bash
# ❌ Wrong
git add . && git commit -m "msg" && git push

# ✅ Correct
rtk git add . && rtk git commit -m "msg" && rtk git push
```

## RTK Commands by Workflow

### Build & Compile (80-90% savings)
```bash
rtk cargo build         # Cargo build output
rtk cargo check         # Cargo check output
rtk cargo clippy        # Clippy warnings grouped by file (80%)
rtk tsc                 # TypeScript errors grouped by file/code (83%)
rtk lint                # ESLint/Biome violations grouped (84%)
rtk prettier --check    # Files needing format only (70%)
rtk next build          # Next.js build with route metrics (87%)
```

### Test (60-99% savings)
```bash
rtk cargo test          # Cargo test failures only (90%)
rtk go test             # Go test failures only (90%)
rtk jest                # Jest failures only (99.5%)
rtk vitest              # Vitest failures only (99.5%)
rtk playwright test     # Playwright failures only (94%)
rtk pytest              # Python test failures only (90%)
rtk rake test           # Ruby test failures only (90%)
rtk rspec               # RSpec test failures only (60%)
rtk test <cmd>          # Generic test wrapper - failures only
```

### Git (59-80% savings)
```bash
rtk git status          # Compact status
rtk git log             # Compact log (works with all git flags)
rtk git diff            # Compact diff (80%)
rtk git show            # Compact show (80%)
rtk git add             # Ultra-compact confirmations (59%)
rtk git commit          # Ultra-compact confirmations (59%)
rtk git push            # Ultra-compact confirmations
rtk git pull            # Ultra-compact confirmations
rtk git branch          # Compact branch list
rtk git fetch           # Compact fetch
rtk git stash           # Compact stash
rtk git worktree        # Compact worktree
```

Note: Git passthrough works for ALL subcommands, even those not explicitly listed.

### GitHub (26-87% savings)
```bash
rtk gh pr view <num>    # Compact PR view (87%)
rtk gh pr checks        # Compact PR checks (79%)
rtk gh run list         # Compact workflow runs (82%)
rtk gh issue list       # Compact issue list (80%)
rtk gh api              # Compact API responses (26%)
```

### JavaScript/TypeScript Tooling (70-90% savings)
```bash
rtk pnpm list           # Compact dependency tree (70%)
rtk pnpm outdated       # Compact outdated packages (80%)
rtk pnpm install        # Compact install output (90%)
rtk npm run <script>    # Compact npm script output
rtk npx <cmd>           # Compact npx command output
rtk prisma              # Prisma without ASCII art (88%)
rtk uv run <cmd>        # Compact uv project command output
```

### Files & Search (60-75% savings)
```bash
rtk ls <path>           # Tree format, compact (65%)
rtk read <file>         # Code reading with filtering (60%)
rtk grep <pattern>      # Search grouped by file (75%). Format flags (-c, -l, -L, -o, -Z) run raw.
rtk find <pattern>      # Find grouped by directory (70%)
```

### Analysis & Debug (70-90% savings)
```bash
rtk err <cmd>           # Filter errors only from any command
rtk log <file>          # Deduplicated logs with counts
rtk json <file>         # JSON structure without values
rtk deps                # Dependency overview
rtk env                 # Environment variables compact
rtk summary <cmd>       # Smart summary of command output
rtk diff                # Ultra-compact diffs
```

### Infrastructure (85% savings)
```bash
rtk docker ps           # Compact container list
rtk docker images       # Compact image list
rtk docker logs <c>     # Deduplicated logs
rtk kubectl get         # Compact resource list
rtk kubectl logs        # Deduplicated pod logs
```

### Network (65-70% savings)
```bash
rtk curl <url>          # Compact HTTP responses (70%)
rtk wget <url>          # Compact download output (65%)
```

### Meta Commands
```bash
rtk gain                # View token savings statistics
rtk gain --history      # View command history with savings
rtk discover            # Analyze Claude Code sessions for missed RTK usage
rtk proxy <cmd>         # Run command without filtering (for debugging)
rtk init                # Add RTK instructions to CLAUDE.md
rtk init --global       # Add RTK to ~/.claude/CLAUDE.md
```

## Token Savings Overview

| Category | Commands | Typical Savings |
|----------|----------|-----------------|
| Tests | vitest, playwright, cargo test | 90-99% |
| Build | next, tsc, lint, prettier | 70-87% |
| Git | status, log, diff, add, commit | 59-80% |
| GitHub | gh pr, gh run, gh issue | 26-87% |
| Package Managers | pnpm, npm, npx | 70-90% |
| Files | ls, read, grep, find | 60-75% |
| Infrastructure | docker, kubectl | 85% |
| Network | curl, wget | 65-70% |

Overall average: **60-90% token reduction** on common development operations.
<!-- /rtk-instructions -->