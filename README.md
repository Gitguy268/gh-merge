<div align="center">

<img src="site/favicon.svg" width="80" height="80" alt="">

# gh merge

**Merge every open pull request GitHub can merge, in one repo or all of them.**

[Install](#install) · [Use](#use) · [AI coding agents](#ai-coding-agents) · [Flags](#flags) · [How it decides](#how-it-decides) · [Web page](https://gitguy268.github.io/gh-merge/)

[![CI](https://github.com/Gitguy268/gh-merge/actions/workflows/ci.yml/badge.svg)](https://github.com/Gitguy268/gh-merge/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/Gitguy268/gh-merge?sort=semver)](https://github.com/Gitguy268/gh-merge/releases)
[![Go](https://img.shields.io/github/go-mod/go-version/Gitguy268/gh-merge)](go.mod)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue)](LICENSE)

<img src="docs/demo.gif" width="860" alt="A terminal: gh merge --dry-run lists the open pull requests of a repository with would-merge or would-skip next to each, then gh merge asks for confirmation and merges them one by one.">

</div>

`gh merge` is an extension for the [GitHub CLI](https://github.com/cli/cli). It lists the open pull requests in a repository, or in every repository you own, shows you what it will do, and merges them one at a time. Drafts and pull requests with conflicts are skipped. CI status is ignored: if GitHub accepts the merge, it happens.

You can run it yourself, or an AI coding agent such as Claude Code, Codex or opencode can run it for you. It never asks a question when there's no terminal, it has JSON output, and it ships a skill file that tells agents how to use it safely.

## Install

| | |
| --- | --- |
| gh extension | `gh extension install Gitguy268/gh-merge` |
| Homebrew (macOS, Linux) | `brew install gitguy268/tap/gh-merge` installs the `gh-merge` command (new Homebrew asks for `brew trust gitguy268/tap` once first) |
| Go 1.26+ | `go install github.com/Gitguy268/gh-merge@latest` |
| From source | `git clone https://github.com/Gitguy268/gh-merge && cd gh-merge && go build && gh extension install .` |

You need [gh](https://cli.github.com) logged in (`gh auth login`). gh merge uses the same login. The binary also works on its own as `gh-merge`, with the same flags. That's what Homebrew and `go install` give you, because gh only runs extensions it installed itself. For `gh merge`, use `gh extension install`.

> Only one extension called `merge` can be installed at a time. If you already have another `gh-merge` extension, remove it first with `gh extension remove merge`.

## Use

```sh
gh merge --dry-run          # what would happen in the repo in this folder
gh merge                    # show the list, ask, then merge
gh merge -R owner/repo      # another repo
gh merge --all              # every repo you own (archived repos are skipped)
gh merge --owner my-org     # every repo of a user or organization
```

Common cases:

```sh
gh merge --all --author app/dependabot --rebase-dependabot   # clear the Dependabot backlog
gh merge --label ready -m rebase -d                          # labelled PRs, rebase, delete branches
gh merge --auto                                              # can't merge yet? turn on auto-merge
gh merge --all --yes --json                                  # scripts and CI
```

## AI coding agents

gh merge works the same with or without an agent. These rules keep an agent from hanging or merging by surprise:

- **No terminal, no prompt.** When stdin or stdout isn't a terminal (an agent's shell, CI, a pipe), gh merge never asks. Without `--yes` it prints the plan and exits with code 2, merging nothing.
- **`--json`** returns every pull request with an `action` (`would-merge`, `would-skip`, `merged`, `auto-merge`, `skipped`, `failed`) and a `reason`, plus totals.
- **Exit codes** tell the agent how it went: 0 done, 1 error, 2 not confirmed, 3 some PRs were skipped or failed.

Teach your agent the tool with one command:

```sh
gh merge install-skill            # every agent found: Claude Code, Codex, opencode, ~/.agents
gh merge install-skill claude     # or name one: claude, codex, opencode, agents, all
gh merge install-skill --print    # print SKILL.md for any other tool
```

The skill tells the agent to run `--dry-run --json` first, show you the list, and only use `--yes` once you agree. Tools that read `AGENTS.md` (Codex, opencode, Cursor and others) pick up the same rules from this repo's [AGENTS.md](AGENTS.md).

<details>
<summary>Claude Code says the merge needs permission, or refuses it</summary>

Merging many pull requests at once can't be undone, so Claude Code's permission system may stop it, especially in auto mode. That's on purpose, and gh merge doesn't try to get around it. You can:

- run it yourself from the Claude Code prompt: `! gh merge --yes` (the `!` runs the command in your own shell);
- approve the command when Claude Code asks;
- or allow it for good in `.claude/settings.json`: `"permissions": {"allow": ["Bash(gh merge:*)"]}`.

</details>

## Flags

| Flag | What it does |
| --- | --- |
| `-R`, `--repo OWNER/NAME` | One repository. The default is the repo in the current folder. |
| `--all` | Every repository you own. |
| `--owner NAME` | Every repository of this user or organization. |
| `--include-forks` | With `--all` or `--owner`, include forks too. |
| `--author LOGIN` | Only PRs by this author. `app/dependabot`, `dependabot[bot]` and `dependabot` all work. Repeatable. |
| `--label NAME` | Only PRs with this label. Repeatable, and all must match. |
| `--base BRANCH` | Only PRs into this branch. |
| `-L`, `--limit N` | At most N PRs per repo (default 100). |
| `-m`, `--method M` | `merge`, `squash` or `rebase`. The default is what the repo allows, trying squash first, then merge, then rebase. |
| `--auto` | When GitHub refuses a merge (required checks or reviews), turn on auto-merge instead. |
| `-d`, `--delete-branch` | Delete the head branch after merging. Branches in forks are left alone. |
| `--rebase-dependabot` | Comment `@dependabot rebase` on Dependabot PRs that have conflicts. |
| `-n`, `--dry-run` | Show what would happen and change nothing. |
| `-y`, `--yes` | Merge without asking. |
| `--json` | JSON output. Never asks, so it needs `--yes` to merge. |

`gh merge version` prints the version, and `gh merge --help` prints the flags.

## How it decides

| GitHub says | gh merge |
| --- | --- |
| Draft | skips it (doesn't count as a problem in the exit code) |
| Conflicts (`CONFLICTING`, `DIRTY`) | skips it, and with `--rebase-dependabot` asks Dependabot to rebase |
| Not computed yet (`UNKNOWN`) | asks again, up to 5 times over about 10 seconds, then tries anyway |
| Failing or pending checks (`UNSTABLE`) | merges |
| Branch protection (`BLOCKED`), behind base (`BEHIND`) | tries, and if GitHub refuses, reports why (or turns on auto-merge with `--auto`) |
| Anything else | merges |

<details>
<summary>Why one at a time, and why it re-checks</summary>

Merging one pull request often puts the next one into conflict. Dependabot updates that touch the same lockfile are the usual case. So after each merge in a repository, gh merge reads the next pull request's state again before merging it, and skips it if it now conflicts. Run gh merge again once Dependabot has rebased, or use `--rebase-dependabot` to ask it to.

</details>

<details>
<summary>JSON output</summary>

```json
{
  "dryRun": false,
  "confirmed": true,
  "repos": ["owner/app"],
  "results": [
    {
      "repo": "owner/app",
      "number": 14,
      "title": "Bump astro from 7.4.7 to 7.4.13",
      "url": "https://github.com/owner/app/pull/14",
      "author": "dependabot",
      "method": "squash",
      "action": "merged"
    }
  ],
  "errors": [],
  "totals": { "merged": 1, "autoMerge": 0, "skipped": 0, "failed": 0, "wouldMerge": 0, "wouldSkip": 0 }
}
```

`errors` lists repositories that couldn't be read.

</details>

<details>
<summary>How it talks to GitHub</summary>

gh merge uses the GitHub GraphQL API through [go-gh](https://github.com/cli/go-gh), the library gh itself uses for extensions. Auth, the GitHub host (`GH_HOST`, GitHub Enterprise) and `GH_TOKEN` work the same as in gh. It sends one request per repository to list pull requests (repositories with none cost nothing extra), one per merge, and a few more to re-check state. It doesn't clone anything or touch your local git checkout.

</details>

## Contributing

Bug reports and pull requests are welcome. See [CONTRIBUTING.md](CONTRIBUTING.md). Report security problems privately: see [SECURITY.md](SECURITY.md).

## License

[MIT](LICENSE) © Lucas Van De Weerd. Dependencies and their licences are listed in [NOTICE](NOTICE). This project is not affiliated with GitHub.
