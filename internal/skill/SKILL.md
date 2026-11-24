---
name: gh-merge
description: Merge all open GitHub pull requests in a repo, or across every repo of a user or org, with the `gh merge` CLI. Use when asked to merge open PRs, merge Dependabot PRs, clear a PR backlog, or turn on auto-merge for many PRs.
---

# gh-merge

`gh merge` is a gh CLI extension that merges every open pull request GitHub can merge. It skips drafts and PRs with conflicts, and it ignores CI status.

Install it with `gh extension install Gitguy268/gh-merge`. You need gh logged in (`gh auth status`).

## Always preview first

Merging can't be undone. Run a dry run, show the user the list, and wait for them to agree before merging.

```sh
gh merge --dry-run --json                 # current repo
gh merge --all --dry-run --json           # every repo the user owns
gh merge --owner ORG --dry-run --json     # every repo of an org
```

Once the user agrees, run the same command with `--yes` in place of `--dry-run`.

When there's no terminal (an agent, CI or a pipe), `gh merge` never prompts. Without `--yes` it only prints the plan and exits with 2.

## Useful flags

| Flag | Effect |
| --- | --- |
| `-R owner/name` | target one repo |
| `--author app/dependabot` | only PRs by this author (repeatable) |
| `--label NAME` | only PRs with this label (repeatable, all must match) |
| `--base main` | only PRs into this branch |
| `-m squash\|merge\|rebase` | merge method (default: the repo's allowed one, squash first) |
| `--auto` | when GitHub refuses a merge (required checks or reviews), turn on auto-merge instead |
| `-d` | delete the head branch after merging |
| `--rebase-dependabot` | comment `@dependabot rebase` on conflicted Dependabot PRs |
| `--json` | machine-readable output |

## Reading the result

The JSON has `results[]`, and each result has `repo`, `number`, `title`, `url`, `action` and `reason`. The `action` is one of `would-merge`, `would-skip`, `merged`, `auto-merge`, `skipped` or `failed`. `totals` holds the counts.

Exit codes:
- 0: done, or nothing to do
- 1: error
- 2: nothing merged because there was no `--yes`, or the user said no
- 3: some PRs were skipped or failed (drafts don't count)

Merging one PR can put the next one into conflict. When that happens, tell the user which PRs were left, and suggest `--rebase-dependabot` for Dependabot PRs.

## If your harness blocks the merge

Some agent harnesses refuse bulk merges in auto mode. Don't try to work around that. Give the user the exact command and ask them to run it themselves (in Claude Code: `! gh merge --yes ...`).
