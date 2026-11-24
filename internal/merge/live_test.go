//go:build live

package merge_test

// Checks the mutation documents against the real GitHub GraphQL API without
// changing anything: every call uses an ID that doesn't exist, and GitHub
// validates the query before it resolves IDs. Run: go test -tags live ./internal/merge
import (
	"strings"
	"testing"

	"github.com/Gitguy268/gh-merge/internal/merge"
	"github.com/cli/go-gh/v2/pkg/api"
)

func TestLiveMutationsAreValid(t *testing.T) {
	c, err := api.DefaultGraphQLClient()
	if err != nil {
		t.Skip("no gh login:", err)
	}
	const bogus = "PR_kwDOdoesnotexist000"
	calls := map[string]error{
		"mergePullRequest (squash)":  merge.MergePR(c, bogus, "squash"),
		"mergePullRequest (rebase)":  merge.MergePR(c, bogus, "rebase"),
		"enablePullRequestAutoMerge": merge.EnableAutoMerge(c, bogus, "merge"),
		"deleteRef":                  merge.DeleteRef(c, "REF_kwDOdoesnotexist000"),
		"addComment":                 merge.Comment(c, bogus, "test"),
	}
	for name, err := range calls {
		if err == nil {
			t.Errorf("%s: succeeded with a bogus ID", name)
			continue
		}
		msg := err.Error()
		if !strings.Contains(msg, "Could not resolve to a node") && !strings.Contains(msg, "Could not resolve to") {
			t.Errorf("%s: unexpected error (broken query?): %s", name, msg)
		} else {
			t.Logf("%s: ok (%s)", name, msg)
		}
	}
	if _, err := merge.GetPR(c, bogus); err == nil {
		t.Error("GetPR with a bogus ID returned no error")
	}
}
