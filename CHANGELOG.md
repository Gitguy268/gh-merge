# Changelog

All notable changes to gh-merge are listed here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow
[Semantic Versioning](https://semver.org/).

## [0.1.0] - 2026-11-24

First release.

- `gh merge` merges every open pull request GitHub can merge in the current repo, one repo (`-R`), every repo you own (`--all`) or every repo of a user or org (`--owner`). Archived repos are skipped, and so are forks unless you pass `--include-forks`.
- Skips drafts and pull requests with conflicts, and ignores CI status. Asks GitHub again when mergeability hasn't been computed yet, and re-checks each pull request after a merge in the same repo.
- Filters: `--author`, `--label`, `--base`, `--limit`.
- Merge options: `--method` (the default is what the repo allows), `--auto` for auto-merge when a merge is refused, `--delete-branch`, `--rebase-dependabot`.
- Safe for AI coding agents and scripts: never prompts without a terminal, `--dry-run`, `--yes`, `--json`, documented exit codes.
- `gh merge install-skill` installs a SKILL.md for Claude Code, Codex, opencode and `~/.agents`, and `AGENTS.md` covers tools that read that file.

[0.1.0]: https://github.com/Gitguy268/gh-merge/releases/tag/v0.1.0
