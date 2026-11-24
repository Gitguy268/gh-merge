# Security policy

## Reporting a vulnerability

Please report security problems privately, not in a public issue.

Use GitHub's private reporting form: <https://github.com/Gitguy268/gh-merge/security/advisories/new>

Include what you found, how to reproduce it, and your OS and gh version. You should get a reply within a week. Please give the fix time to ship before you publish details.

## Scope

gh merge acts on GitHub with your gh login, so the most useful reports are about:

- gh merge merging, commenting on or deleting something it wasn't asked to (a filter that matches too much, a branch deleted from the wrong repository);
- a way to make it merge without `--yes` or a confirmed prompt;
- a pull request title, branch name or other repository data that gets text into your terminal unfiltered;
- `install-skill` writing outside the skill folders it names;
- the GitHub Actions workflows in this repository.

## Supported versions

Only the latest release and the `main` branch are supported.
