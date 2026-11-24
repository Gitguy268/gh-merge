//go:build demo

// Built only with `-tags demo` to record the README pictures against a fake
// GitHub, so no real pull request is ever merged. Release builds don't have it.
package main

import (
	"strings"
	"time"

	"github.com/Gitguy268/gh-merge/internal/fakegh"
	"github.com/Gitguy268/gh-merge/internal/merge"
	"github.com/cli/go-gh/v2/pkg/repository"
)

type slowDoer struct{ s *fakegh.Server }

func (d slowDoer) Do(q string, v map[string]interface{}, r interface{}) error {
	if strings.HasPrefix(q, "mutation") {
		time.Sleep(450 * time.Millisecond)
	} else {
		time.Sleep(60 * time.Millisecond)
	}
	return d.s.Do(q, v, r)
}

func init() {
	s := &fakegh.Server{
		Login: "you",
		Repos: []*fakegh.Repo{
			{Owner: "you", Name: "webapp", Squash: true, Merge: true},
			{Owner: "you", Name: "cli", Squash: true},
			{Owner: "you", Name: "docs", Merge: true},
		},
		PRs: []*fakegh.PR{
			{Repo: "you/webapp", Number: 41, Title: "Bump vite from 8.3.5 to 8.3.8", Author: "dependabot", Labels: []string{"dependencies"}},
			{Repo: "you/webapp", Number: 42, Title: "Bump eslint from 10.14.0 to 10.15.0", Author: "dependabot", Labels: []string{"dependencies"}, MergeState: "UNSTABLE"},
			{Repo: "you/webapp", Number: 43, Title: "Bump globals from 17.13.0 to 17.14.0", Author: "dependabot", Labels: []string{"dependencies"}, DirtyAfter: 41},
			{Repo: "you/webapp", Number: 44, Title: "New settings page", Author: "you", Draft: true, MergeState: "DRAFT"},
			{Repo: "you/cli", Number: 12, Title: "Bump golang.org/x/term from 0.47.0 to 0.48.0", Author: "dependabot", Labels: []string{"dependencies"}, UnknownReads: 1},
			{Repo: "you/cli", Number: 13, Title: "Fix --json output on Windows", Author: "sam", CrossRepo: true},
			{Repo: "you/docs", Number: 7, Title: "Fix typo in the install guide", Author: "alex", CrossRepo: true},
		},
	}
	demoApp = func(a *app) {
		a.newClient = func(string) (merge.Doer, error) { return slowDoer{s}, nil }
		a.currentRepo = func() (repository.Repository, error) {
			return repository.Repository{Host: "github.com", Owner: "you", Name: "webapp"}, nil
		}
		a.sleep = func(time.Duration) { time.Sleep(300 * time.Millisecond) }
	}
}
