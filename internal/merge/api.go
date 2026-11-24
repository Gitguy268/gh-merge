// Package merge finds open pull requests and merges them through the
// GitHub GraphQL API.
package merge

import (
	"fmt"
	"strings"
)

// Doer runs one GraphQL request. *api.GraphQLClient from go-gh satisfies it.
type Doer interface {
	Do(query string, variables map[string]interface{}, response interface{}) error
}

// Repo is a repository and the merge methods it allows.
type Repo struct {
	Owner         string
	Name          string
	Archived      bool
	Fork          bool
	OpenPRs       int
	CanWrite      bool // the viewer can merge here (WRITE, MAINTAIN or ADMIN)
	SquashAllowed bool
	MergeAllowed  bool
	RebaseAllowed bool
}

// FullName returns owner/name.
func (r Repo) FullName() string { return r.Owner + "/" + r.Name }

// PR is an open pull request.
type PR struct {
	ID         string
	Number     int
	Title      string
	URL        string
	Author     string
	IsDraft    bool
	Mergeable  string // MERGEABLE, CONFLICTING, UNKNOWN
	MergeState string // CLEAN, DIRTY, BLOCKED, BEHIND, UNSTABLE, HAS_HOOKS, DRAFT, UNKNOWN
	State      string // OPEN, CLOSED, MERGED
	BaseRef    string
	HeadRef    string
	HeadRefID  string
	CrossRepo  bool
	Labels     []string
}

const repoFields = `name owner{login} isArchived isFork viewerPermission
  squashMergeAllowed mergeCommitAllowed rebaseMergeAllowed
  pullRequests(states:OPEN){totalCount}`

type repoNode struct {
	Name               string
	Owner              struct{ Login string }
	IsArchived         bool
	IsFork             bool
	ViewerPermission   string
	SquashMergeAllowed bool
	MergeCommitAllowed bool
	RebaseMergeAllowed bool
	PullRequests       struct{ TotalCount int }
}

func (n repoNode) repo() Repo {
	return Repo{
		Owner: n.Owner.Login, Name: n.Name,
		Archived: n.IsArchived, Fork: n.IsFork, OpenPRs: n.PullRequests.TotalCount,
		CanWrite:      n.ViewerPermission == "WRITE" || n.ViewerPermission == "MAINTAIN" || n.ViewerPermission == "ADMIN",
		SquashAllowed: n.SquashMergeAllowed, MergeAllowed: n.MergeCommitAllowed, RebaseAllowed: n.RebaseMergeAllowed,
	}
}

// GetRepo loads one repository.
func GetRepo(c Doer, owner, name string) (Repo, error) {
	var resp struct{ Repository *repoNode }
	q := `query($owner:String!,$name:String!){repository(owner:$owner,name:$name){` + repoFields + `}}`
	if err := c.Do(q, map[string]interface{}{"owner": owner, "name": name}, &resp); err != nil {
		return Repo{}, err
	}
	if resp.Repository == nil {
		return Repo{}, fmt.Errorf("repository %s/%s not found", owner, name)
	}
	return resp.Repository.repo(), nil
}

// Viewer returns the login of the authenticated user.
func Viewer(c Doer) (string, error) {
	var resp struct{ Viewer struct{ Login string } }
	if err := c.Do(`query{viewer{login}}`, nil, &resp); err != nil {
		return "", err
	}
	return resp.Viewer.Login, nil
}

// OwnerRepos lists every repository owned by a user or organization.
func OwnerRepos(c Doer, owner string) ([]Repo, error) {
	q := `query($owner:String!,$after:String){repositoryOwner(login:$owner){
  repositories(first:100,after:$after,ownerAffiliations:OWNER,orderBy:{field:NAME,direction:ASC}){
    nodes{` + repoFields + `} pageInfo{hasNextPage endCursor}}}}`
	var out []Repo
	var after interface{}
	for {
		var resp struct {
			RepositoryOwner *struct {
				Repositories struct {
					Nodes    []repoNode
					PageInfo struct {
						HasNextPage bool
						EndCursor   string
					}
				}
			}
		}
		if err := c.Do(q, map[string]interface{}{"owner": owner, "after": after}, &resp); err != nil {
			return nil, err
		}
		if resp.RepositoryOwner == nil {
			return nil, fmt.Errorf("user or organization %q not found", owner)
		}
		for _, n := range resp.RepositoryOwner.Repositories.Nodes {
			out = append(out, n.repo())
		}
		pi := resp.RepositoryOwner.Repositories.PageInfo
		if !pi.HasNextPage {
			return out, nil
		}
		after = pi.EndCursor
	}
}

const prFields = `id number title url isDraft mergeable mergeStateStatus state
  baseRefName headRefName headRef{id} isCrossRepository author{login}
  labels(first:30){nodes{name}}`

type prNode struct {
	ID                string
	Number            int
	Title             string
	URL               string
	IsDraft           bool
	Mergeable         string
	MergeStateStatus  string
	State             string
	BaseRefName       string
	HeadRefName       string
	HeadRef           *struct{ ID string }
	IsCrossRepository bool
	Author            *struct{ Login string }
	Labels            struct{ Nodes []struct{ Name string } }
}

func (n prNode) pr() PR {
	p := PR{
		ID: n.ID, Number: n.Number, Title: n.Title, URL: n.URL, IsDraft: n.IsDraft,
		Mergeable: n.Mergeable, MergeState: n.MergeStateStatus, State: n.State,
		BaseRef: n.BaseRefName, HeadRef: n.HeadRefName, CrossRepo: n.IsCrossRepository,
	}
	if n.HeadRef != nil {
		p.HeadRefID = n.HeadRef.ID
	}
	if n.Author != nil {
		p.Author = n.Author.Login
	} else {
		p.Author = "ghost"
	}
	for _, l := range n.Labels.Nodes {
		p.Labels = append(p.Labels, l.Name)
	}
	return p
}

// OpenPRs lists up to limit open pull requests, oldest first.
func OpenPRs(c Doer, r Repo, limit int) ([]PR, error) {
	q := `query($owner:String!,$name:String!,$first:Int!,$after:String){repository(owner:$owner,name:$name){
  pullRequests(states:OPEN,first:$first,after:$after,orderBy:{field:CREATED_AT,direction:ASC}){
    nodes{` + prFields + `} pageInfo{hasNextPage endCursor}}}}`
	var out []PR
	var after interface{}
	for len(out) < limit {
		first := min(limit-len(out), 100)
		var resp struct {
			Repository *struct {
				PullRequests struct {
					Nodes    []prNode
					PageInfo struct {
						HasNextPage bool
						EndCursor   string
					}
				}
			}
		}
		vars := map[string]interface{}{"owner": r.Owner, "name": r.Name, "first": first, "after": after}
		if err := c.Do(q, vars, &resp); err != nil {
			return nil, err
		}
		if resp.Repository == nil {
			return nil, fmt.Errorf("repository %s not found", r.FullName())
		}
		for _, n := range resp.Repository.PullRequests.Nodes {
			out = append(out, n.pr())
		}
		pi := resp.Repository.PullRequests.PageInfo
		if !pi.HasNextPage {
			break
		}
		after = pi.EndCursor
	}
	return out, nil
}

// GetPR reloads a pull request by node ID.
func GetPR(c Doer, id string) (PR, error) {
	var resp struct{ Node *prNode }
	q := `query($id:ID!){node(id:$id){... on PullRequest{` + prFields + `}}}`
	if err := c.Do(q, map[string]interface{}{"id": id}, &resp); err != nil {
		return PR{}, err
	}
	if resp.Node == nil {
		return PR{}, fmt.Errorf("pull request %s not found", id)
	}
	return resp.Node.pr(), nil
}

// MergePR merges a pull request now.
func MergePR(c Doer, id, method string) error {
	var resp struct {
		MergePullRequest struct{ PullRequest struct{ Merged bool } }
	}
	q := `mutation($id:ID!,$method:PullRequestMergeMethod!){mergePullRequest(input:{pullRequestId:$id,mergeMethod:$method}){pullRequest{merged}}}`
	if err := c.Do(q, map[string]interface{}{"id": id, "method": strings.ToUpper(method)}, &resp); err != nil {
		return err
	}
	if !resp.MergePullRequest.PullRequest.Merged {
		return fmt.Errorf("GitHub did not merge the pull request")
	}
	return nil
}

// EnableAutoMerge turns on auto-merge, so GitHub merges once requirements pass.
func EnableAutoMerge(c Doer, id, method string) error {
	var resp map[string]interface{}
	q := `mutation($id:ID!,$method:PullRequestMergeMethod!){enablePullRequestAutoMerge(input:{pullRequestId:$id,mergeMethod:$method}){clientMutationId}}`
	return c.Do(q, map[string]interface{}{"id": id, "method": strings.ToUpper(method)}, &resp)
}

// DeleteRef deletes a branch by ref node ID.
func DeleteRef(c Doer, refID string) error {
	var resp map[string]interface{}
	q := `mutation($id:ID!){deleteRef(input:{refId:$id}){clientMutationId}}`
	return c.Do(q, map[string]interface{}{"id": refID}, &resp)
}

// Comment adds a comment to a pull request.
func Comment(c Doer, id, body string) error {
	var resp map[string]interface{}
	q := `mutation($id:ID!,$body:String!){addComment(input:{subjectId:$id,body:$body}){clientMutationId}}`
	return c.Do(q, map[string]interface{}{"id": id, "body": body}, &resp)
}
