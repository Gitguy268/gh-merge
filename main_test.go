package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Gitguy268/gh-merge/internal/fakegh"
	"github.com/Gitguy268/gh-merge/internal/merge"
	"github.com/cli/go-gh/v2/pkg/repository"
)

func newApp(t *testing.T, s *fakegh.Server, interactive bool, stdin string) (*app, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	var out, errOut bytes.Buffer
	return &app{
		stdin:       strings.NewReader(stdin),
		stdout:      &out,
		stderr:      &errOut,
		interactive: interactive,
		home:        t.TempDir(),
		newClient:   func(string) (merge.Doer, error) { return s, nil },
		currentRepo: func() (repository.Repository, error) {
			return repository.Repository{Host: "github.com", Owner: "me", Name: "app"}, nil
		},
		sleep: func(time.Duration) {},
	}, &out, &errOut
}

func fixture() *fakegh.Server {
	return &fakegh.Server{
		Login: "me",
		Repos: []*fakegh.Repo{
			{Owner: "me", Name: "app", Squash: true},
			{Owner: "me", Name: "lib", Merge: true},
			{Owner: "me", Name: "old", Archived: true, Merge: true},
			{Owner: "me", Name: "forked", Fork: true, Merge: true},
			{Owner: "me", Name: "readonly", ReadOnly: true, Merge: true},
		},
		PRs: []*fakegh.PR{
			{Repo: "me/app", Number: 1, Title: "Bump a", Author: "dependabot"},
			{Repo: "me/app", Number: 2, Title: "WIP", Author: "me", Draft: true, MergeState: "DRAFT"},
			{Repo: "me/lib", Number: 7, Title: "Fix", Author: "me"},
			{Repo: "me/old", Number: 1, Title: "Old", Author: "me"},
			{Repo: "me/forked", Number: 1, Title: "Fork", Author: "me"},
			{Repo: "me/readonly", Number: 1, Title: "Nope", Author: "me"},
		},
	}
}

func TestNoTerminalWithoutYesDoesNothing(t *testing.T) {
	s := fixture()
	a, out, errOut := newApp(t, s, false, "")
	if code := a.run(nil); code != exitNotConfirmed {
		t.Fatalf("exit %d, want %d\n%s%s", code, exitNotConfirmed, out, errOut)
	}
	if len(s.Merged) != 0 {
		t.Fatalf("merged without --yes: %v", s.Merged)
	}
	if !strings.Contains(out.String(), "would-merge") || !strings.Contains(errOut.String(), "--yes") {
		t.Errorf("want plan on stdout and a --yes hint on stderr\n%s\n%s", out, errOut)
	}
}

func TestYesMergesCurrentRepo(t *testing.T) {
	s := fixture()
	a, out, _ := newApp(t, s, false, "")
	if code := a.run([]string{"--yes"}); code != exitOK {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if len(s.Merged) != 1 || s.Merged[0] != "me/app#1 SQUASH" {
		t.Errorf("merged = %v", s.Merged)
	}
	if !strings.Contains(out.String(), "1 merged, 1 skipped") {
		t.Errorf("summary missing:\n%s", out)
	}
}

func TestAllSkipsArchivedAndForks(t *testing.T) {
	s := fixture()
	a, out, _ := newApp(t, s, false, "")
	if code := a.run([]string{"--all", "-y"}); code != exitOK {
		t.Fatalf("exit %d\n%s", code, out)
	}
	want := "me/app#1 SQUASH,me/lib#7 MERGE"
	if got := strings.Join(s.Merged, ","); got != want {
		t.Errorf("merged = %s, want %s", got, want)
	}

	s = fixture()
	a, _, _ = newApp(t, s, false, "")
	a.run([]string{"--owner", "me", "--include-forks", "-y"})
	if len(s.Merged) != 3 {
		t.Errorf("with --include-forks merged = %v", s.Merged)
	}
}

func TestReadOnlyRepos(t *testing.T) {
	s := fixture()
	a, _, errOut := newApp(t, s, false, "")
	if code := a.run([]string{"-R", "me/readonly", "--dry-run"}); code != exitError || !strings.Contains(errOut.String(), "write access") {
		t.Errorf("single read-only repo: exit %d, %q", code, errOut)
	}
	errOut.Reset()
	a.run([]string{"--all", "--dry-run"})
	if !strings.Contains(errOut.String(), "Skipping 1 repositories") {
		t.Errorf("--all should report skipped read-only repos: %q", errOut)
	}
}

func TestDryRunChangesNothing(t *testing.T) {
	s := fixture()
	a, out, _ := newApp(t, s, true, "")
	if code := a.run([]string{"--all", "--dry-run", "--yes"}); code != exitOK {
		t.Fatalf("exit %d", code)
	}
	if len(s.Merged) != 0 {
		t.Fatalf("dry run merged %v", s.Merged)
	}
	if !strings.Contains(out.String(), "Dry run: 2 to merge, 1 to skip.") {
		t.Errorf("output:\n%s", out)
	}
}

func TestInteractivePrompt(t *testing.T) {
	s := fixture()
	a, out, _ := newApp(t, s, true, "n\n")
	if code := a.run(nil); code != exitNotConfirmed || len(s.Merged) != 0 {
		t.Fatalf("answer n: exit %d, merged %v", code, s.Merged)
	}
	if !strings.Contains(out.String(), "Merge 1 pull request(s) in 1 repo(s)? [y/N]") {
		t.Errorf("prompt missing:\n%s", out)
	}

	a, _, _ = newApp(t, s, true, "y\n")
	if code := a.run(nil); code != exitOK || len(s.Merged) != 1 {
		t.Fatalf("answer y: exit %d, merged %v", code, s.Merged)
	}
}

func TestJSON(t *testing.T) {
	s := fixture()
	a, out, _ := newApp(t, s, true, "y\n") // --json never prompts, even in a terminal
	if code := a.run([]string{"--json", "-R", "me/app"}); code != exitNotConfirmed {
		t.Fatalf("exit %d", code)
	}
	var r merge.Report
	if err := json.Unmarshal(out.Bytes(), &r); err != nil {
		t.Fatalf("bad JSON: %v\n%s", err, out)
	}
	if r.Confirmed || len(r.Results) != 2 || r.Totals.WouldMerge != 1 || len(s.Merged) != 0 {
		t.Errorf("report = %+v", r)
	}

	out.Reset()
	if code := a.run([]string{"--json", "--yes", "-R", "me/app"}); code != exitOK {
		t.Fatalf("exit %d", code)
	}
	if err := json.Unmarshal(out.Bytes(), &r); err != nil || r.Totals.Merged != 1 || !r.Confirmed {
		t.Errorf("report = %+v, %v", r, err)
	}
}

func TestIncompleteExitCode(t *testing.T) {
	s := fixture()
	s.PRs[0].MergeError = "Pull request is in clean status"
	a, out, _ := newApp(t, s, false, "")
	if code := a.run([]string{"-y", "-R", "me/app"}); code != exitIncomplete {
		t.Fatalf("exit %d, want %d\n%s", code, exitIncomplete, out)
	}
}

func TestFlagErrors(t *testing.T) {
	for _, args := range [][]string{
		{"--all", "--owner", "x"},
		{"--repo", "a/b", "--all"},
		{"--method", "fast"},
		{"--limit", "0"},
		{"stray"},
		{"--nope"},
	} {
		a, _, errOut := newApp(t, fixture(), false, "")
		if code := a.run(args); code != exitError || errOut.Len() == 0 {
			t.Errorf("%v: exit %d, stderr %q", args, code, errOut)
		}
	}
}

func TestInstallSkill(t *testing.T) {
	a, out, _ := newApp(t, fixture(), false, "")
	if code := a.run([]string{"install-skill"}); code != exitError {
		t.Errorf("no agents installed: exit %d, want error", code)
	}
	os.MkdirAll(filepath.Join(a.home, ".claude"), 0o755)
	if code := a.run([]string{"install-skill", "--dry-run"}); code != exitOK || !strings.Contains(out.String(), ".claude") {
		t.Fatalf("dry run: exit %d\n%s", code, out)
	}
	if _, err := os.Stat(filepath.Join(a.home, ".claude", "skills")); err == nil {
		t.Fatal("dry run wrote files")
	}
	if code := a.run([]string{"install-skill"}); code != exitOK {
		t.Fatalf("install: exit %d", code)
	}
	b, err := os.ReadFile(filepath.Join(a.home, ".claude", "skills", "gh-merge", "SKILL.md"))
	if err != nil || !bytes.HasPrefix(b, []byte("---\nname: gh-merge\n")) {
		t.Fatalf("SKILL.md: %v %q", err, b[:min(len(b), 40)])
	}
	if code := a.run([]string{"install-skill", "codex", "opencode"}); code != exitOK {
		t.Fatal("named install failed")
	}
	for _, p := range []string{".codex/skills/gh-merge/SKILL.md", ".config/opencode/skills/gh-merge/SKILL.md"} {
		if _, err := os.Stat(filepath.Join(a.home, p)); err != nil {
			t.Errorf("missing %s", p)
		}
	}
	if code := a.run([]string{"install-skill", "vim"}); code != exitError {
		t.Error("unknown agent should fail")
	}
}
