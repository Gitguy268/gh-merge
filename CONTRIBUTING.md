# Contributing

Thanks for helping with gh merge.

By taking part you agree to the [Code of Conduct](CODE_OF_CONDUCT.md).

## Reporting a bug or asking for a feature

Open an [issue](https://github.com/Gitguy268/gh-merge/issues/new/choose) and use the template. For a bug, include the command and its `--dry-run --json` output (remove anything private), your gh version and OS, and any repo settings that matter: branch protection, merge queue, allowed merge methods.

Report security problems privately: see [SECURITY.md](SECURITY.md).

## Making a change

1. Fork the repository and create a branch from `main`.
2. Build it and install your build as the extension. You need Go 1.26 or newer and gh.

   ```sh
   go build
   gh extension install .     # gh merge now runs your build
   gh merge --dry-run -R owner/repo
   ```

   Use `--dry-run` while you work. Don't merge real pull requests to test a change; the tests use an in-memory fake instead.

3. Make your change. Keep commits small, each with a clear message.
4. Run the checks below. If you changed flags or output, update README.md, AGENTS.md and internal/skill/SKILL.md.
5. Open a pull request and fill in the template.

## Checks

```sh
gofmt -l .          # prints nothing
go vet ./...
go test -race ./...
go test -tags live ./internal/merge   # optional: checks the mutations against real GitHub with IDs that don't exist, so nothing changes
```

## Layout

| Path | What |
| --- | --- |
| `main.go` | Flags, the prompt, exit codes, `install-skill` and `version` |
| `internal/merge` | Listing repos and pull requests, deciding what to do with each, merging, reporting |
| `internal/fakegh` | An in-memory stand-in for the GitHub GraphQL API, used by the tests |
| `internal/skill` | The SKILL.md for AI coding agents (embedded in the binary) and its installer |
| `site/` | The web page |
