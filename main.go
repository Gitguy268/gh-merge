// Command gh-merge merges all open pull requests that GitHub can merge.
// Run it as a gh extension (`gh merge`) or on its own (`gh-merge`).
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"strings"
	"time"

	"github.com/Gitguy268/gh-merge/internal/merge"
	"github.com/Gitguy268/gh-merge/internal/skill"
	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/cli/go-gh/v2/pkg/repository"
	"github.com/cli/go-gh/v2/pkg/term"
)

// Exit codes.
const (
	exitOK           = 0
	exitError        = 1
	exitNotConfirmed = 2
	exitIncomplete   = 3
)

var version = ""

// demoApp is set only in builds with -tags demo (see demo.go).
var demoApp func(*app)

const usage = `Merge every open pull request GitHub can merge.

USAGE
  gh merge [flags]                 merge open PRs in the current repo
  gh merge --all [flags]           ... in every repo you own
  gh merge --owner NAME [flags]    ... in every repo of a user or org
  gh merge install-skill [AGENT]   add the gh-merge skill for AI coding agents
  gh merge version

Drafts and PRs with conflicts are skipped. CI status is ignored.

TARGET
  -R, --repo OWNER/NAME   one repository (default: the repo in this folder)
      --all               every repository you own (archived ones are skipped)
      --owner NAME        every repository of this user or organization
      --include-forks     with --all/--owner, include forks

FILTER
      --author LOGIN      only PRs by this author, e.g. app/dependabot (repeatable)
      --label NAME        only PRs with this label (repeatable; all must match)
      --base BRANCH       only PRs into this branch
  -L, --limit N           at most N PRs per repo (default 100)

MERGE
  -m, --method METHOD     merge, squash or rebase (default: what the repo allows, squash first)
      --auto              if GitHub refuses a merge, turn on auto-merge instead
  -d, --delete-branch     delete the head branch after merging
      --rebase-dependabot comment "@dependabot rebase" on conflicted Dependabot PRs

OUTPUT
  -n, --dry-run           show what would happen; change nothing
  -y, --yes               merge without asking
      --json              JSON output (never asks; needs --yes to merge)

Without a terminal (AI agents, CI, pipes) gh merge never asks: it prints the
plan and exits 2 unless --yes is given.

EXIT CODES
  0  done, or nothing to do
  1  error
  2  nothing merged: not confirmed
  3  some PRs were skipped or failed (drafts don't count)
`

type multiFlag []string

func (m *multiFlag) String() string     { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error { *m = append(*m, v); return nil }

// app holds everything that touches the outside world, so tests can swap it.
type app struct {
	stdin       io.Reader
	stdout      io.Writer
	stderr      io.Writer
	interactive bool // can ask the user a question
	color       bool
	home        string
	newClient   func(host string) (merge.Doer, error)
	currentRepo func() (repository.Repository, error)
	sleep       func(time.Duration)
}

func main() {
	t := term.FromEnv()
	home, _ := os.UserHomeDir()
	a := &app{
		stdin:       os.Stdin,
		stdout:      t.Out(),
		stderr:      t.ErrOut(),
		interactive: t.IsTerminalOutput() && term.IsTerminal(os.Stdin),
		color:       t.IsColorEnabled(),
		home:        home,
		newClient: func(host string) (merge.Doer, error) {
			return api.NewGraphQLClient(api.ClientOptions{Host: host})
		},
		currentRepo: repository.Current,
		sleep:       time.Sleep,
	}
	if demoApp != nil {
		demoApp(a)
	}
	os.Exit(a.run(os.Args[1:]))
}

func (a *app) run(args []string) int {
	if len(args) > 0 {
		switch args[0] {
		case "install-skill":
			return a.installSkill(args[1:])
		case "version", "--version":
			fmt.Fprintln(a.stdout, "gh-merge", buildVersion())
			return exitOK
		case "help":
			fmt.Fprint(a.stdout, usage)
			return exitOK
		}
	}

	fs := flag.NewFlagSet("gh merge", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var (
		repoFlag, owner, base, method   string
		all, forks, auto, del, rebaseDB bool
		dryRun, yes, asJSON             bool
		limit                           int
		authors, labels                 multiFlag
	)
	fs.StringVar(&repoFlag, "repo", "", "")
	fs.StringVar(&repoFlag, "R", "", "")
	fs.BoolVar(&all, "all", false, "")
	fs.StringVar(&owner, "owner", "", "")
	fs.BoolVar(&forks, "include-forks", false, "")
	fs.Var(&authors, "author", "")
	fs.Var(&labels, "label", "")
	fs.StringVar(&base, "base", "", "")
	fs.IntVar(&limit, "limit", 100, "")
	fs.IntVar(&limit, "L", 100, "")
	fs.StringVar(&method, "method", "", "")
	fs.StringVar(&method, "m", "", "")
	fs.BoolVar(&auto, "auto", false, "")
	fs.BoolVar(&del, "delete-branch", false, "")
	fs.BoolVar(&del, "d", false, "")
	fs.BoolVar(&rebaseDB, "rebase-dependabot", false, "")
	fs.BoolVar(&dryRun, "dry-run", false, "")
	fs.BoolVar(&dryRun, "n", false, "")
	fs.BoolVar(&yes, "yes", false, "")
	fs.BoolVar(&yes, "y", false, "")
	fs.BoolVar(&asJSON, "json", false, "")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(a.stdout, usage)
			return exitOK
		}
		fmt.Fprintf(a.stderr, "gh merge: %v\nRun 'gh merge --help' for usage.\n", err)
		return exitError
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(a.stderr, "gh merge: unexpected argument %q\nRun 'gh merge --help' for usage.\n", fs.Arg(0))
		return exitError
	}
	targets := 0
	for _, set := range []bool{repoFlag != "", all, owner != ""} {
		if set {
			targets++
		}
	}
	if targets > 1 {
		fmt.Fprintln(a.stderr, "gh merge: use only one of --repo, --all and --owner")
		return exitError
	}
	if limit < 1 {
		fmt.Fprintln(a.stderr, "gh merge: --limit must be at least 1")
		return exitError
	}
	if method != "" {
		switch strings.ToLower(method) {
		case "merge", "squash", "rebase":
		default:
			fmt.Fprintf(a.stderr, "gh merge: unknown merge method %q (use merge, squash or rebase)\n", method)
			return exitError
		}
	}

	style := merge.Style{Color: a.color && !asJSON}
	client, repos, err := a.resolveRepos(repoFlag, all, owner, forks)
	if err != nil {
		fmt.Fprintf(a.stderr, "gh merge: %v\n", err)
		return exitError
	}

	opts := merge.Options{
		Filter:           merge.Filter{Authors: authors, Labels: labels, Base: base},
		Method:           method,
		Auto:             auto,
		DeleteBranch:     del,
		RebaseDependabot: rebaseDB,
		Limit:            limit,
		Polls:            5,
		PollDelay:        2 * time.Second,
		Sleep:            a.sleep,
	}
	if !asJSON && len(repos) > 1 {
		fmt.Fprintf(a.stderr, "Checking %d repositories...\n", len(repos))
	}
	items, repoErrs := merge.Plan(client, repos, opts)
	for _, e := range repoErrs {
		fmt.Fprintf(a.stderr, "gh merge: %s: %s\n", e.Repo, e.Error)
	}
	names := make([]string, 0, len(repos))
	for _, r := range repos {
		names = append(names, r.FullName())
	}
	report := merge.Report{DryRun: dryRun, Repos: names, Errors: repoErrs}
	preview := merge.Preview(items)
	t := merge.Count(preview)
	reposWithWork := map[string]bool{}
	for _, r := range preview {
		if r.Action == merge.ActWouldMerge {
			reposWithWork[r.Repo] = true
		}
	}

	finish := func(results []merge.Result, confirmed bool, code int) int {
		report.Results, report.Totals, report.Confirmed = results, merge.Count(results), confirmed
		if asJSON {
			if err := merge.WriteJSON(a.stdout, report); err != nil {
				fmt.Fprintf(a.stderr, "gh merge: %v\n", err)
				return exitError
			}
		}
		if code == exitOK && len(repoErrs) > 0 {
			return exitError
		}
		return code
	}

	if len(items) == 0 {
		if !asJSON {
			fmt.Fprintln(a.stdout, "No open pull requests found.")
		}
		return finish(nil, yes, exitOK)
	}

	planCode := exitOK
	if merge.Unmerged(asSkipped(preview)) {
		planCode = exitIncomplete
	}

	if dryRun {
		if !asJSON {
			merge.Table(a.stdout, style, preview)
			fmt.Fprintf(a.stdout, "\nDry run: %s.\n", t)
		}
		return finish(preview, false, exitOK)
	}

	if t.WouldMerge == 0 && !rebaseDB {
		if !asJSON {
			merge.Table(a.stdout, style, preview)
			fmt.Fprintln(a.stdout, "\nNothing GitHub can merge right now.")
		}
		return finish(preview, yes, planCode)
	}

	if !yes {
		if asJSON || !a.interactive {
			if !asJSON {
				merge.Table(a.stdout, style, preview)
			}
			fmt.Fprintf(a.stderr, "\nNot merging: no --yes given and no terminal to ask. Run again with --yes to merge %d pull request(s).\n", t.WouldMerge)
			return finish(preview, false, exitNotConfirmed)
		}
		merge.Table(a.stdout, style, preview)
		fmt.Fprintf(a.stdout, "\nMerge %d pull request(s) in %d repo(s)? [y/N] ", t.WouldMerge, len(reposWithWork))
		line, _ := bufio.NewReader(a.stdin).ReadString('\n')
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "y", "yes":
		default:
			fmt.Fprintln(a.stdout, "Nothing merged.")
			return exitNotConfirmed
		}
	}

	if !asJSON {
		opts.Progress = func(r merge.Result) { fmt.Fprintln(a.stdout, merge.Line(style, r)) }
	}
	results := merge.Execute(client, items, opts)
	if !asJSON {
		fmt.Fprintf(a.stdout, "\n%s.\n", merge.Count(results))
	}
	code := exitOK
	if merge.Unmerged(results) {
		code = exitIncomplete
	}
	return finish(results, true, code)
}

// asSkipped maps would-skip to skipped so Unmerged can judge a preview.
func asSkipped(rs []merge.Result) []merge.Result {
	out := make([]merge.Result, len(rs))
	for i, r := range rs {
		if r.Action == merge.ActWouldSkip {
			r.Action = merge.ActSkipped
		}
		out[i] = r
	}
	return out
}

func (a *app) resolveRepos(repoFlag string, all bool, owner string, forks bool) (merge.Doer, []merge.Repo, error) {
	if all || owner != "" {
		c, err := a.newClient("")
		if err != nil {
			return nil, nil, err
		}
		if all {
			if owner, err = merge.Viewer(c); err != nil {
				return nil, nil, err
			}
		}
		list, err := merge.OwnerRepos(c, owner)
		if err != nil {
			return nil, nil, err
		}
		var out []merge.Repo
		readOnly := 0
		for _, r := range list {
			if r.Archived || (r.Fork && !forks) {
				continue
			}
			if !r.CanWrite {
				readOnly++
				continue
			}
			out = append(out, r)
		}
		if readOnly > 0 {
			fmt.Fprintf(a.stderr, "Skipping %d repositories you can't merge in (no write access).\n", readOnly)
		}
		return c, out, nil
	}

	var r repository.Repository
	var err error
	if repoFlag != "" {
		r, err = repository.Parse(repoFlag)
	} else {
		r, err = a.currentRepo()
		if err != nil {
			err = errors.New("this folder is not a git repository with a GitHub remote; use -R OWNER/NAME, --all or --owner")
		}
	}
	if err != nil {
		return nil, nil, err
	}
	c, err := a.newClient(r.Host)
	if err != nil {
		return nil, nil, err
	}
	repo, err := merge.GetRepo(c, r.Owner, r.Name)
	if err != nil {
		return nil, nil, err
	}
	if repo.Archived {
		return nil, nil, fmt.Errorf("%s is archived", repo.FullName())
	}
	if !repo.CanWrite {
		return nil, nil, fmt.Errorf("you don't have write access to %s, so you can't merge there", repo.FullName())
	}
	return c, []merge.Repo{repo}, nil
}

func (a *app) installSkill(args []string) int {
	fs := flag.NewFlagSet("install-skill", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var dryRun, print bool
	fs.BoolVar(&dryRun, "dry-run", false, "")
	fs.BoolVar(&dryRun, "n", false, "")
	fs.BoolVar(&print, "print", false, "")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprint(a.stdout, `Install the gh-merge skill so AI coding agents know how to use gh merge.

USAGE
  gh merge install-skill [claude|codex|opencode|agents|all ...] [--dry-run] [--print]

With no agent named, it installs for every agent whose config folder exists:
  claude    ~/.claude/skills/gh-merge/SKILL.md
  codex     ~/.codex/skills/gh-merge/SKILL.md
  opencode  ~/.config/opencode/skills/gh-merge/SKILL.md
  agents    ~/.agents/skills/gh-merge/SKILL.md

  --print     write SKILL.md to stdout instead (for any other tool)
  --dry-run   show where it would go
`)
			return exitOK
		}
		fmt.Fprintf(a.stderr, "gh merge install-skill: %v\n", err)
		return exitError
	}
	if print {
		a.stdout.Write(skill.Content)
		return exitOK
	}
	targets, err := skill.Select(a.home, fs.Args())
	if err != nil {
		fmt.Fprintf(a.stderr, "gh merge install-skill: %v\n", err)
		return exitError
	}
	if dryRun {
		for _, t := range targets {
			fmt.Fprintf(a.stdout, "would write %s\n", t.Dir+string(os.PathSeparator)+"SKILL.md")
		}
		return exitOK
	}
	paths, err := skill.Install(targets)
	for _, p := range paths {
		fmt.Fprintf(a.stdout, "wrote %s\n", p)
	}
	if err != nil {
		fmt.Fprintf(a.stderr, "gh merge install-skill: %v\n", err)
		return exitError
	}
	return exitOK
}

func buildVersion() string {
	if version != "" {
		return version
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "dev"
	}
	if v := info.Main.Version; v != "" && v != "(devel)" {
		return v
	}
	for _, s := range info.Settings {
		if s.Key == "vcs.revision" && len(s.Value) >= 7 {
			return "dev-" + s.Value[:7]
		}
	}
	return "dev"
}
