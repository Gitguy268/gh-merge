// Package fakegh is an in-memory stand-in for the GitHub GraphQL API, used
// by tests. It answers only the queries gh-merge sends.
package fakegh

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// PR is a fake pull request.
type PR struct {
	Repo       string // owner/name
	Number     int
	Title      string
	Author     string
	Draft      bool
	Mergeable  string
	MergeState string
	CrossRepo  bool
	Labels     []string
	Base       string
	// UnknownReads makes the first N reads report UNKNOWN mergeability.
	UnknownReads int
	// MergeError makes mergePullRequest fail with this message.
	MergeError string
	// DirtyAfter makes this PR conflict once the PR with this number in the same repo is merged.
	DirtyAfter int

	state     string
	headRefID string
}

// Repo is a fake repository.
type Repo struct {
	Owner, Name           string
	Archived, Fork        bool
	ReadOnly              bool // the viewer has READ permission only
	Squash, Merge, Rebase bool
}

// Server holds the fake state and records mutations.
type Server struct {
	mu       sync.Mutex
	Login    string
	Repos    []*Repo
	PRs      []*PR
	Merged   []string // "owner/name#n method"
	Auto     []string
	Deleted  []string
	Comments []string
	AutoErr  string
}

func id(p *PR) string { return fmt.Sprintf("PR_%s_%d", p.Repo, p.Number) }

func (s *Server) find(prID string) *PR {
	for _, p := range s.PRs {
		if id(p) == prID {
			return p
		}
	}
	return nil
}

func (s *Server) open(repo string) []*PR {
	var out []*PR
	for _, p := range s.PRs {
		if p.Repo == repo && p.state == "" {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Number < out[j].Number })
	return out
}

func (s *Server) repoJSON(r *Repo) map[string]any {
	return map[string]any{
		"name": r.Name, "owner": map[string]any{"login": r.Owner},
		"isArchived": r.Archived, "isFork": r.Fork, "viewerPermission": perm(r),
		"squashMergeAllowed": r.Squash, "mergeCommitAllowed": r.Merge, "rebaseMergeAllowed": r.Rebase,
		"pullRequests": map[string]any{"totalCount": len(s.open(r.Owner + "/" + r.Name))},
	}
}

func perm(r *Repo) string {
	if r.ReadOnly {
		return "READ"
	}
	return "ADMIN"
}

func (s *Server) prJSON(p *PR) map[string]any {
	mergeable, ms := p.Mergeable, p.MergeState
	if mergeable == "" {
		mergeable = "MERGEABLE"
	}
	if ms == "" {
		ms = "CLEAN"
	}
	if p.UnknownReads > 0 {
		p.UnknownReads--
		mergeable, ms = "UNKNOWN", "UNKNOWN"
	}
	state := "OPEN"
	if p.state != "" {
		state = p.state
	}
	labels := []map[string]any{}
	for _, l := range p.Labels {
		labels = append(labels, map[string]any{"name": l})
	}
	base := p.Base
	if base == "" {
		base = "main"
	}
	var head any
	if p.headRefID != "gone" {
		head = map[string]any{"id": "REF_" + id(p)}
	}
	return map[string]any{
		"id": id(p), "number": p.Number, "title": p.Title,
		"url":     fmt.Sprintf("https://github.com/%s/pull/%d", p.Repo, p.Number),
		"isDraft": p.Draft, "mergeable": mergeable, "mergeStateStatus": ms, "state": state,
		"baseRefName": base, "headRefName": fmt.Sprintf("branch-%d", p.Number), "headRef": head,
		"isCrossRepository": p.CrossRepo, "author": map[string]any{"login": p.Author},
		"labels": map[string]any{"nodes": labels},
	}
}

// Do implements merge.Doer.
func (s *Server) Do(query string, vars map[string]interface{}, response interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	str := func(k string) string { v, _ := vars[k].(string); return v }
	var data any
	switch {
	case strings.Contains(query, "viewer{login}"):
		data = map[string]any{"viewer": map[string]any{"login": s.Login}}
	case strings.Contains(query, "repositoryOwner"):
		nodes := []any{}
		for _, r := range s.Repos {
			if r.Owner == str("owner") {
				nodes = append(nodes, s.repoJSON(r))
			}
		}
		if len(nodes) == 0 {
			data = map[string]any{"repositoryOwner": nil}
		} else {
			data = map[string]any{"repositoryOwner": map[string]any{"repositories": map[string]any{
				"nodes": nodes, "pageInfo": map[string]any{"hasNextPage": false, "endCursor": ""}}}}
		}
	case strings.Contains(query, "repository(owner"):
		full := str("owner") + "/" + str("name")
		var repo *Repo
		for _, r := range s.Repos {
			if r.Owner+"/"+r.Name == full {
				repo = r
			}
		}
		if repo == nil {
			data = map[string]any{"repository": nil}
			break
		}
		if strings.Contains(query, "pullRequests(states:OPEN,first") {
			nodes := []any{}
			for _, p := range s.open(full) {
				nodes = append(nodes, s.prJSON(p))
			}
			data = map[string]any{"repository": map[string]any{"pullRequests": map[string]any{
				"nodes": nodes, "pageInfo": map[string]any{"hasNextPage": false, "endCursor": ""}}}}
		} else {
			data = map[string]any{"repository": s.repoJSON(repo)}
		}
	case strings.Contains(query, "node(id"):
		p := s.find(str("id"))
		if p == nil {
			data = map[string]any{"node": nil}
		} else {
			data = map[string]any{"node": s.prJSON(p)}
		}
	case strings.Contains(query, "mergePullRequest"):
		p := s.find(str("id"))
		if p == nil {
			return errors.New("GraphQL: Could not resolve to a node (mergePullRequest)")
		}
		if p.MergeError != "" {
			return fmt.Errorf("GraphQL: %s (mergePullRequest)", p.MergeError)
		}
		if p.Mergeable == "CONFLICTING" || p.Draft {
			return errors.New("GraphQL: Pull Request is not mergeable (mergePullRequest)")
		}
		p.state = "MERGED"
		s.Merged = append(s.Merged, fmt.Sprintf("%s#%d %s", p.Repo, p.Number, str("method")))
		for _, o := range s.PRs {
			if o.Repo == p.Repo && o.DirtyAfter == p.Number {
				o.Mergeable, o.MergeState = "CONFLICTING", "DIRTY"
				o.UnknownReads = 1
			}
		}
		data = map[string]any{"mergePullRequest": map[string]any{"pullRequest": map[string]any{"merged": true}}}
	case strings.Contains(query, "enablePullRequestAutoMerge"):
		if s.AutoErr != "" {
			return fmt.Errorf("GraphQL: %s (enablePullRequestAutoMerge)", s.AutoErr)
		}
		p := s.find(str("id"))
		s.Auto = append(s.Auto, fmt.Sprintf("%s#%d", p.Repo, p.Number))
		data = map[string]any{}
	case strings.Contains(query, "deleteRef"):
		s.Deleted = append(s.Deleted, str("id"))
		data = map[string]any{}
	case strings.Contains(query, "addComment"):
		p := s.find(str("id"))
		s.Comments = append(s.Comments, fmt.Sprintf("%s#%d %s", p.Repo, p.Number, str("body")))
		data = map[string]any{}
	default:
		return fmt.Errorf("fakegh: unexpected query %q", query)
	}
	b, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, response)
}
