package merge

import (
	"fmt"
	"strings"
	"time"
)

// Options controls planning and merging.
type Options struct {
	Filter           Filter
	Method           string // "" picks the repo's preferred allowed method
	Auto             bool   // enable auto-merge when a merge is refused
	DeleteBranch     bool
	RebaseDependabot bool
	Limit            int // max PRs per repo
	Polls            int // times to re-check UNKNOWN mergeability
	PollDelay        time.Duration
	Sleep            func(time.Duration) // nil means time.Sleep
	Progress         func(Result)        // called after each PR while merging
}

func (o Options) sleep(d time.Duration) {
	if o.Sleep != nil {
		o.Sleep(d)
		return
	}
	time.Sleep(d)
}

// Item is one pull request in the plan.
type Item struct {
	Repo     Repo
	PR       PR
	Method   string
	Decision Decision
	Reason   string
}

// Result actions.
const (
	ActWouldMerge = "would-merge"
	ActWouldSkip  = "would-skip"
	ActMerged     = "merged"
	ActAutoMerge  = "auto-merge"
	ActSkipped    = "skipped"
	ActFailed     = "failed"
)

// Result is what happened (or would happen) to one pull request.
type Result struct {
	Repo   string `json:"repo"`
	Number int    `json:"number"`
	Title  string `json:"title"`
	URL    string `json:"url"`
	Author string `json:"author"`
	Method string `json:"method,omitempty"`
	Action string `json:"action"`
	Reason string `json:"reason,omitempty"`
}

func result(it Item, action, reason string) Result {
	return Result{
		Repo: it.Repo.FullName(), Number: it.PR.Number, Title: it.PR.Title, URL: it.PR.URL,
		Author: it.PR.Author, Method: it.Method, Action: action, Reason: reason,
	}
}

// RepoError records a repository that could not be read.
type RepoError struct {
	Repo  string `json:"repo"`
	Error string `json:"error"`
}

// Plan lists the open pull requests in repos and decides what to do with each.
// Repositories with no open pull requests cost no extra request.
func Plan(c Doer, repos []Repo, o Options) ([]Item, []RepoError) {
	var items []Item
	var errs []RepoError
	for _, r := range repos {
		if r.OpenPRs == 0 {
			continue
		}
		method, methodErr := PickMethod(r, o.Method)
		prs, err := OpenPRs(c, r, o.Limit)
		if err != nil {
			errs = append(errs, RepoError{Repo: r.FullName(), Error: err.Error()})
			continue
		}
		for _, p := range prs {
			if !o.Filter.Match(p) {
				continue
			}
			it := Item{Repo: r, PR: p, Method: method}
			if methodErr != nil {
				it.Decision, it.Reason = Skip, methodErr.Error()
			} else {
				it.Decision, it.Reason = Classify(p)
			}
			items = append(items, it)
		}
	}
	resolvePending(c, items, o)
	return items, errs
}

// resolvePending re-reads pull requests whose mergeability GitHub has not
// computed yet. Asking for the field starts the computation, so a short wait
// usually settles it. Ones that stay unknown are attempted anyway.
func resolvePending(c Doer, items []Item, o Options) {
	for try := 0; try < o.Polls; try++ {
		waiting := false
		for i := range items {
			if items[i].Decision == Pending {
				waiting = true
			}
		}
		if !waiting {
			return
		}
		o.sleep(o.PollDelay)
		for i := range items {
			if items[i].Decision != Pending {
				continue
			}
			p, err := GetPR(c, items[i].PR.ID)
			if err != nil {
				continue
			}
			items[i].PR = p
			items[i].Decision, items[i].Reason = Classify(p)
		}
	}
	for i := range items {
		if items[i].Decision == Pending {
			items[i].Decision, items[i].Reason = Attempt, "mergeability unknown; trying anyway"
		}
	}
}

// Preview turns a plan into would-merge / would-skip results.
func Preview(items []Item) []Result {
	out := make([]Result, 0, len(items))
	for _, it := range items {
		if it.Decision == Skip {
			out = append(out, result(it, ActWouldSkip, it.Reason))
		} else {
			out = append(out, result(it, ActWouldMerge, it.Reason))
		}
	}
	return out
}

// Execute merges the plan one pull request at a time. After a merge in a
// repository, the next pull request there is re-read, because merging one
// often puts another into conflict (Dependabot lockfile updates, for example).
func Execute(c Doer, items []Item, o Options) []Result {
	out := make([]Result, 0, len(items))
	mergedIn := map[string]bool{}
	for _, it := range items {
		res := executeOne(c, it, mergedIn[it.Repo.FullName()], o)
		if res.Action == ActMerged {
			mergedIn[it.Repo.FullName()] = true
		}
		out = append(out, res)
		if o.Progress != nil {
			o.Progress(res)
		}
	}
	return out
}

func executeOne(c Doer, it Item, stale bool, o Options) Result {
	if it.Decision != Skip && stale {
		if p, err := GetPR(c, it.PR.ID); err == nil {
			it.PR = p
			it.Decision, it.Reason = Classify(p)
			for try := 0; it.Decision == Pending && try < o.Polls; try++ {
				o.sleep(o.PollDelay)
				if p, err = GetPR(c, it.PR.ID); err == nil {
					it.PR = p
					it.Decision, it.Reason = Classify(p)
				}
			}
		}
	}
	if it.Decision == Skip {
		reason := it.Reason
		if reason == "merge conflict" && o.RebaseDependabot && IsDependabot(it.PR) {
			if err := Comment(c, it.PR.ID, "@dependabot rebase"); err == nil {
				reason += "; asked Dependabot to rebase"
			} else {
				reason += "; rebase comment failed: " + err.Error()
			}
		}
		return result(it, ActSkipped, reason)
	}

	err := MergePR(c, it.PR.ID, it.Method)
	if err != nil {
		if o.Auto {
			if aerr := EnableAutoMerge(c, it.PR.ID, it.Method); aerr == nil {
				return result(it, ActAutoMerge, "will merge when requirements pass ("+cleanErr(err)+")")
			} else {
				return result(it, ActFailed, cleanErr(err)+"; auto-merge: "+cleanErr(aerr))
			}
		}
		return result(it, ActFailed, cleanErr(err))
	}

	reason := ""
	if o.DeleteBranch {
		switch {
		case it.PR.CrossRepo:
			reason = "branch is in a fork; not deleted"
		case it.PR.HeadRefID == "":
			reason = "branch already gone"
		default:
			if derr := DeleteRef(c, it.PR.HeadRefID); derr != nil && !alreadyGone(derr) {
				reason = "branch not deleted: " + cleanErr(derr)
			} else {
				reason = "branch deleted"
			}
		}
	}
	return result(it, ActMerged, reason)
}

func alreadyGone(err error) bool {
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "could not resolve") || strings.Contains(s, "not found") || strings.Contains(s, "does not exist")
}

func cleanErr(err error) string {
	s := err.Error()
	s = strings.TrimPrefix(s, "GraphQL: ")
	if i := strings.Index(s, " (mergePullRequest)"); i >= 0 {
		s = s[:i]
	}
	if i := strings.Index(s, " (enablePullRequestAutoMerge)"); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

// Totals counts results by action.
type Totals struct {
	Merged     int `json:"merged"`
	AutoMerge  int `json:"autoMerge"`
	Skipped    int `json:"skipped"`
	Failed     int `json:"failed"`
	WouldMerge int `json:"wouldMerge"`
	WouldSkip  int `json:"wouldSkip"`
}

// Count totals results.
func Count(rs []Result) Totals {
	var t Totals
	for _, r := range rs {
		switch r.Action {
		case ActMerged:
			t.Merged++
		case ActAutoMerge:
			t.AutoMerge++
		case ActSkipped:
			t.Skipped++
		case ActFailed:
			t.Failed++
		case ActWouldMerge:
			t.WouldMerge++
		case ActWouldSkip:
			t.WouldSkip++
		}
	}
	return t
}

// Unmerged reports whether any pull request that should have merged did not.
// Drafts are not counted: they are never meant to be merged.
func Unmerged(rs []Result) bool {
	for _, r := range rs {
		if r.Action == ActFailed || (r.Action == ActSkipped && r.Reason != "draft") {
			return true
		}
	}
	return false
}

func (t Totals) String() string {
	parts := []string{}
	add := func(n int, s string) {
		if n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, s))
		}
	}
	add(t.WouldMerge, "to merge")
	add(t.WouldSkip, "to skip")
	add(t.Merged, "merged")
	add(t.AutoMerge, "set to auto-merge")
	add(t.Skipped, "skipped")
	add(t.Failed, "failed")
	if len(parts) == 0 {
		return "nothing to do"
	}
	return strings.Join(parts, ", ")
}
