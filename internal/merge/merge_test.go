package merge_test

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Gitguy268/gh-merge/internal/fakegh"
	"github.com/Gitguy268/gh-merge/internal/merge"
)

func TestClassify(t *testing.T) {
	cases := []struct {
		pr   merge.PR
		want merge.Decision
		why  string
	}{
		{merge.PR{Mergeable: "MERGEABLE", MergeState: "CLEAN"}, merge.Attempt, ""},
		{merge.PR{Mergeable: "MERGEABLE", MergeState: "UNSTABLE"}, merge.Attempt, "checks failing"},
		{merge.PR{Mergeable: "MERGEABLE", MergeState: "BLOCKED"}, merge.Attempt, "blocked; will try"},
		{merge.PR{Mergeable: "MERGEABLE", MergeState: "BEHIND"}, merge.Attempt, "behind base branch"},
		{merge.PR{IsDraft: true, Mergeable: "MERGEABLE", MergeState: "DRAFT"}, merge.Skip, "draft"},
		{merge.PR{Mergeable: "CONFLICTING", MergeState: "DIRTY"}, merge.Skip, "merge conflict"},
		{merge.PR{Mergeable: "UNKNOWN", MergeState: "UNKNOWN"}, merge.Pending, "mergeability not computed yet"},
		{merge.PR{State: "MERGED"}, merge.Skip, "already merged"},
	}
	for _, c := range cases {
		got, why := merge.Classify(c.pr)
		if got != c.want || why != c.why {
			t.Errorf("Classify(%+v) = %v %q, want %v %q", c.pr, got, why, c.want, c.why)
		}
	}
}

func TestPickMethod(t *testing.T) {
	r := merge.Repo{Owner: "o", Name: "r", MergeAllowed: true, RebaseAllowed: true}
	if m, _ := merge.PickMethod(r, ""); m != "merge" {
		t.Errorf("default = %q, want merge (squash not allowed)", m)
	}
	r.SquashAllowed = true
	if m, _ := merge.PickMethod(r, ""); m != "squash" {
		t.Errorf("default = %q, want squash", m)
	}
	if m, _ := merge.PickMethod(r, "REBASE"); m != "rebase" {
		t.Errorf("requested = %q, want rebase", m)
	}
	r.RebaseAllowed = false
	if _, err := merge.PickMethod(r, "rebase"); err == nil {
		t.Error("expected error for a method the repo does not allow")
	}
	if _, err := merge.PickMethod(merge.Repo{}, ""); err == nil {
		t.Error("expected error when no method is allowed")
	}
}

func TestFilter(t *testing.T) {
	p := merge.PR{Author: "dependabot", Labels: []string{"dependencies", "go"}, BaseRef: "main"}
	for _, f := range []merge.Filter{
		{},
		{Authors: []string{"app/dependabot"}},
		{Authors: []string{"dependabot[bot]"}},
		{Authors: []string{"someone", "Dependabot"}},
		{Labels: []string{"Dependencies"}},
		{Labels: []string{"dependencies", "go"}, Base: "main"},
	} {
		if !f.Match(p) {
			t.Errorf("%+v should match", f)
		}
	}
	for _, f := range []merge.Filter{
		{Authors: []string{"renovate"}},
		{Labels: []string{"dependencies", "python"}},
		{Base: "dev"},
	} {
		if f.Match(p) {
			t.Errorf("%+v should not match", f)
		}
	}
}

func server() *fakegh.Server {
	return &fakegh.Server{
		Login: "me",
		Repos: []*fakegh.Repo{{Owner: "me", Name: "app", Squash: true, Merge: true}},
	}
}

func opts() (merge.Options, *int) {
	sleeps := 0
	return merge.Options{Limit: 100, Polls: 5, PollDelay: time.Second, Sleep: func(time.Duration) { sleeps++ }}, &sleeps
}

func repos(t *testing.T, s *fakegh.Server) []merge.Repo {
	t.Helper()
	r, err := merge.GetRepo(s, "me", "app")
	if err != nil {
		t.Fatal(err)
	}
	return []merge.Repo{r}
}

func actions(rs []merge.Result) []string {
	var out []string
	for _, r := range rs {
		out = append(out, r.Action)
	}
	return out
}

func TestPlanPollsUnknown(t *testing.T) {
	s := server()
	s.PRs = []*fakegh.PR{
		{Repo: "me/app", Number: 1, Author: "dependabot", UnknownReads: 2},
		{Repo: "me/app", Number: 2, Author: "me", Draft: true, MergeState: "DRAFT"},
		{Repo: "me/app", Number: 3, Author: "me", Mergeable: "CONFLICTING", MergeState: "DIRTY"},
	}
	o, sleeps := opts()
	items, errs := merge.Plan(s, repos(t, s), o)
	if len(errs) != 0 {
		t.Fatal(errs)
	}
	got := actions(merge.Preview(items))
	want := []string{merge.ActWouldMerge, merge.ActWouldSkip, merge.ActWouldSkip}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("preview = %v, want %v", got, want)
	}
	if *sleeps != 2 {
		t.Errorf("slept %d times, want 2 (UNKNOWN in the list, UNKNOWN once more, then known)", *sleeps)
	}
	if items[0].Method != "squash" {
		t.Errorf("method = %q, want squash", items[0].Method)
	}
}

func TestPlanStillUnknownIsAttempted(t *testing.T) {
	s := server()
	s.PRs = []*fakegh.PR{{Repo: "me/app", Number: 1, UnknownReads: 100}}
	o, sleeps := opts()
	items, _ := merge.Plan(s, repos(t, s), o)
	if items[0].Decision != merge.Attempt || *sleeps != o.Polls {
		t.Errorf("decision %v after %d sleeps; want Attempt after %d", items[0].Decision, *sleeps, o.Polls)
	}
}

func TestExecuteRechecksAfterMerge(t *testing.T) {
	s := server()
	s.PRs = []*fakegh.PR{
		{Repo: "me/app", Number: 1, Author: "dependabot"},
		{Repo: "me/app", Number: 2, Author: "dependabot", DirtyAfter: 1},
		{Repo: "me/app", Number: 3, Author: "me"},
	}
	o, _ := opts()
	o.RebaseDependabot = true
	items, _ := merge.Plan(s, repos(t, s), o)
	var progress int
	o.Progress = func(merge.Result) { progress++ }
	rs := merge.Execute(s, items, o)
	want := []string{merge.ActMerged, merge.ActSkipped, merge.ActMerged}
	if got := actions(rs); !reflect.DeepEqual(got, want) {
		t.Fatalf("actions = %v, want %v (%+v)", got, want, rs)
	}
	if !strings.Contains(rs[1].Reason, "asked Dependabot to rebase") {
		t.Errorf("reason = %q", rs[1].Reason)
	}
	if len(s.Comments) != 1 || !strings.HasSuffix(s.Comments[0], "#2 @dependabot rebase") {
		t.Errorf("comments = %v", s.Comments)
	}
	if !reflect.DeepEqual(s.Merged, []string{"me/app#1 SQUASH", "me/app#3 SQUASH"}) {
		t.Errorf("merged = %v", s.Merged)
	}
	if progress != 3 {
		t.Errorf("progress called %d times", progress)
	}
	if !merge.Unmerged(rs) {
		t.Error("a conflicted PR should count as unmerged")
	}
}

func TestExecuteRefusedWithAndWithoutAuto(t *testing.T) {
	for _, auto := range []bool{false, true} {
		s := server()
		s.PRs = []*fakegh.PR{{Repo: "me/app", Number: 1, MergeState: "BLOCKED", MergeError: "Required status check \"ci\" is expected"}}
		o, _ := opts()
		o.Auto = auto
		items, _ := merge.Plan(s, repos(t, s), o)
		rs := merge.Execute(s, items, o)
		if auto {
			if rs[0].Action != merge.ActAutoMerge || len(s.Auto) != 1 {
				t.Errorf("auto: %+v, auto calls %v", rs[0], s.Auto)
			}
		} else {
			if rs[0].Action != merge.ActFailed || rs[0].Reason != `Required status check "ci" is expected` {
				t.Errorf("no auto: %+v", rs[0])
			}
		}
	}
}

func TestExecuteAutoMergeAlsoFails(t *testing.T) {
	s := server()
	s.AutoErr = "Auto merge is not allowed for this repository"
	s.PRs = []*fakegh.PR{{Repo: "me/app", Number: 1, MergeError: "Base branch was modified"}}
	o, _ := opts()
	o.Auto = true
	items, _ := merge.Plan(s, repos(t, s), o)
	rs := merge.Execute(s, items, o)
	if rs[0].Action != merge.ActFailed || !strings.Contains(rs[0].Reason, "auto-merge: Auto merge is not allowed") {
		t.Errorf("%+v", rs[0])
	}
}

func TestDeleteBranch(t *testing.T) {
	s := server()
	s.PRs = []*fakegh.PR{
		{Repo: "me/app", Number: 1},
		{Repo: "me/app", Number: 2, CrossRepo: true},
	}
	o, _ := opts()
	o.DeleteBranch = true
	items, _ := merge.Plan(s, repos(t, s), o)
	rs := merge.Execute(s, items, o)
	if rs[0].Reason != "branch deleted" || rs[1].Reason != "branch is in a fork; not deleted" {
		t.Errorf("reasons = %q, %q", rs[0].Reason, rs[1].Reason)
	}
	if len(s.Deleted) != 1 {
		t.Errorf("deleted = %v", s.Deleted)
	}
}

func TestOwnerReposAndViewer(t *testing.T) {
	s := server()
	s.Repos = append(s.Repos, &fakegh.Repo{Owner: "me", Name: "old", Archived: true, Merge: true})
	login, err := merge.Viewer(s)
	if err != nil || login != "me" {
		t.Fatalf("viewer = %q, %v", login, err)
	}
	rs, err := merge.OwnerRepos(s, "me")
	if err != nil || len(rs) != 2 || !rs[1].Archived {
		t.Fatalf("repos = %+v, %v", rs, err)
	}
	if _, err := merge.OwnerRepos(s, "nobody"); err == nil {
		t.Error("expected error for unknown owner")
	}
}

func TestDraftsDoNotCountAsUnmerged(t *testing.T) {
	rs := []merge.Result{{Action: merge.ActMerged}, {Action: merge.ActSkipped, Reason: "draft"}}
	if merge.Unmerged(rs) {
		t.Error("drafts should not make the run incomplete")
	}
	if got := merge.Count(rs).String(); got != "1 merged, 1 skipped" {
		t.Errorf("totals = %q", got)
	}
}

func TestTextOutputStripsControlCharacters(t *testing.T) {
	r := merge.Result{Repo: "me/app", Number: 1, Title: "\x1b]0;pwn\x07Bump\x1b[2J x‮", Author: "evil\x1b[31m", Action: merge.ActMerged, Reason: "a\nb"}
	var buf strings.Builder
	merge.Table(&buf, merge.Style{}, []merge.Result{r})
	out := buf.String() + merge.Line(merge.Style{}, r)
	if strings.ContainsAny(out, "\x1b\x07‮") {
		t.Errorf("control characters reached the terminal: %q", out)
	}
	if !strings.Contains(out, "]0;pwnBump[2J x") || !strings.Contains(out, "(ab)") {
		t.Errorf("text lost: %q", out)
	}
}
