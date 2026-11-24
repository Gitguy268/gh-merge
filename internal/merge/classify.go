package merge

import (
	"fmt"
	"strings"
)

// Filter selects which pull requests to look at.
type Filter struct {
	Authors []string // logins; "app/dependabot" and "dependabot[bot]" both match the bot "dependabot"
	Labels  []string // all must be present
	Base    string
}

func normLogin(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimPrefix(s, "app/")
	return strings.TrimSuffix(s, "[bot]")
}

// Match reports whether a pull request passes the filter.
func (f Filter) Match(p PR) bool {
	if f.Base != "" && p.BaseRef != f.Base {
		return false
	}
	if len(f.Authors) > 0 {
		ok := false
		for _, a := range f.Authors {
			if normLogin(a) == normLogin(p.Author) {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	for _, want := range f.Labels {
		found := false
		for _, l := range p.Labels {
			if strings.EqualFold(l, want) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// IsDependabot reports whether the pull request was opened by Dependabot.
func IsDependabot(p PR) bool { return normLogin(p.Author) == "dependabot" }

// Decision is what to do with a pull request.
type Decision int

const (
	// Attempt means try to merge it.
	Attempt Decision = iota
	// Skip means leave it alone; see the reason.
	Skip
	// Pending means GitHub has not computed mergeability yet.
	Pending
)

// Classify decides what to do with a pull request. CI status is ignored on
// purpose: anything GitHub can merge is attempted.
func Classify(p PR) (Decision, string) {
	switch {
	case p.State != "" && p.State != "OPEN":
		return Skip, "already " + strings.ToLower(p.State)
	case p.IsDraft || p.MergeState == "DRAFT":
		return Skip, "draft"
	case p.Mergeable == "CONFLICTING" || p.MergeState == "DIRTY":
		return Skip, "merge conflict"
	case p.Mergeable == "UNKNOWN" || p.MergeState == "UNKNOWN":
		return Pending, "mergeability not computed yet"
	case p.MergeState == "BLOCKED":
		return Attempt, "blocked; will try"
	case p.MergeState == "BEHIND":
		return Attempt, "behind base branch"
	case p.MergeState == "UNSTABLE":
		return Attempt, "checks failing"
	}
	return Attempt, ""
}

// PickMethod returns the merge method to use for a repository. A requested
// method must be allowed by the repo; otherwise squash, merge, rebase is the
// order of preference.
func PickMethod(r Repo, requested string) (string, error) {
	allowed := map[string]bool{"squash": r.SquashAllowed, "merge": r.MergeAllowed, "rebase": r.RebaseAllowed}
	if requested != "" {
		requested = strings.ToLower(requested)
		ok, known := allowed[requested]
		if !known {
			return "", fmt.Errorf("unknown merge method %q (use merge, squash or rebase)", requested)
		}
		if !ok {
			return "", fmt.Errorf("%s does not allow %s merges", r.FullName(), requested)
		}
		return requested, nil
	}
	for _, m := range []string{"squash", "merge", "rebase"} {
		if allowed[m] {
			return m, nil
		}
	}
	return "", fmt.Errorf("%s allows no merge method", r.FullName())
}
