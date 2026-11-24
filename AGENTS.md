# AGENTS.md

This file is for AI coding agents (Codex, opencode, Cursor, Claude Code and others). It covers two things: using gh merge, and working on this repo.

## Using gh merge to merge pull requests

gh merge merges every open pull request GitHub can merge. Merging can't be undone, so:

1. **Preview first:** `gh merge --dry-run --json` (add `-R owner/name`, `--all` or `--owner NAME` as needed).
2. Show the user which pull requests would merge and which would be skipped.
3. **Merge only after the user agrees:** run the same command with `--yes` in place of `--dry-run`.

Without a terminal, gh merge never prompts. Without `--yes` it exits with 2 and changes nothing.

Exit codes:
- 0: done
- 1: error
- 2: not confirmed
- 3: some PRs were skipped (conflicts) or failed

When you get 3, report the skipped and failed PRs with their `reason`.

If your harness refuses the merge, don't work around it. Give the user the exact command to run themselves.

The full guide is in [internal/skill/SKILL.md](internal/skill/SKILL.md). Install it as a skill with `gh merge install-skill`.

## Working on this repo

- Go 1.26+, a single binary, and `github.com/cli/go-gh/v2` as the only direct dependency.
- `main.go` holds the flags, prompt, exit codes and `install-skill`.
- `internal/merge` lists repos and PRs, classifies each PR, merges and reports.
- `internal/fakegh` is an in-memory GitHub GraphQL fake for tests.
- `internal/skill` holds the embedded SKILL.md and the installer.
- Checks: `gofmt -l .` must print nothing, `go vet ./...`, `go test -race ./...`.
- Tests never call GitHub. Don't merge real pull requests while testing; use `--dry-run`.
- When you change flags or output, update README.md, this file and internal/skill/SKILL.md together.
