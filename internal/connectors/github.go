package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// GitHub: issues and pull requests through the REST API, and repository events through webhooks.
type GitHub struct{}

func (GitHub) Name() string        { return "github" }
func (GitHub) DisplayName() string { return "GitHub" }
func (GitHub) Description() string {
	return "Read and manage issues and pull requests; get notified about repository events."
}
func (GitHub) CredentialFields() []Field {
	return []Field{
		{Key: "token", Label: "Personal access token", Secret: true,
			Help: "A fine-grained token with access to the repositories agents should use (Issues and Pull requests: read & write)."},
		{Key: "webhook_secret", Label: "Webhook secret", Secret: true, Optional: true,
			Help: "Set the same secret on the repository's webhook (Settings → Webhooks) to receive events."},
	}
}
func (GitHub) ConfigFields() []Field {
	return []Field{{Key: "base_url", Label: "API URL", Optional: true, Help: "Only for GitHub Enterprise, e.g. https://github.example.com/api/v3"}}
}
func (GitHub) SignalTypes() []SignalType {
	common := []string{"repo", "action", "title", "number", "author", "url", "body"}
	return []SignalType{
		{Type: "github.issues", Description: "An issue was opened, closed, edited, labeled, assigned…", Fields: append(common, "labels", "assignee")},
		{Type: "github.issue_comment", Description: "Someone commented on an issue or pull request", Fields: common},
		{Type: "github.pull_request", Description: "A pull request was opened, closed, merged, reviewed…", Fields: append(common, "merged", "base", "head")},
		{Type: "github.push", Description: "Commits were pushed", Fields: []string{"repo", "ref", "author", "commits", "url"}},
		{Type: "github.workflow_run", Description: "A GitHub Actions run finished", Fields: []string{"repo", "action", "name", "conclusion", "branch", "url"}},
	}
}

func (GitHub) Tools(context.Context, Account) ([]Tool, error) {
	return []Tool{
		{Name: "list_issues", Description: "List issues in a repository (newest first).",
			Parameters: params(map[string]string{"repo": "owner/name", "?state": "open (default), closed or all", "?labels": "comma-separated labels"})},
		{Name: "get_issue", Description: "Get an issue or pull request with its description and recent comments.",
			Parameters: params(map[string]string{"repo": "owner/name", "number": "int:issue or PR number"})},
		{Name: "create_issue", Description: "Open an issue.", External: true,
			Parameters: params(map[string]string{"repo": "owner/name", "title": "title", "?body": "Markdown description", "?labels": "comma-separated labels"})},
		{Name: "comment", Description: "Comment on an issue or pull request.", External: true,
			Parameters: params(map[string]string{"repo": "owner/name", "number": "int:issue or PR number", "body": "Markdown comment"})},
		{Name: "close_issue", Description: "Close an issue.", External: true,
			Parameters: params(map[string]string{"repo": "owner/name", "number": "int:issue number"})},
		{Name: "list_pull_requests", Description: "List pull requests in a repository.",
			Parameters: params(map[string]string{"repo": "owner/name", "?state": "open (default), closed or all"})},
	}, nil
}

func (g GitHub) api(ctx context.Context, acct Account, method, path string, body, out any) error {
	h := http.Header{}
	h.Set("Authorization", "Bearer "+acct.Credentials["token"])
	h.Set("X-GitHub-Api-Version", "2022-11-28")
	return apiRequest(ctx, method, baseURL(acct, "https://api.github.com")+path, h, body, out)
}

func validRepo(repo string) error {
	parts := strings.Split(repo, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" || strings.ContainsAny(repo, "?#% ") {
		return userErr("repo must look like owner/name, got %q", repo)
	}
	return nil
}

func (g GitHub) Call(ctx context.Context, acct Account, tool string, args json.RawMessage) (any, error) {
	var a struct {
		Repo   string `json:"repo"`
		Number int    `json:"number"`
		State  string `json:"state"`
		Labels string `json:"labels"`
		Title  string `json:"title"`
		Body   string `json:"body"`
	}
	if err := decodeArgs(args, &a); err != nil {
		return nil, err
	}
	if err := validRepo(a.Repo); err != nil {
		return nil, err
	}
	repo := "/repos/" + a.Repo
	switch tool {
	case "list_issues", "list_pull_requests":
		q := url.Values{"per_page": {"30"}}
		if a.State != "" {
			q.Set("state", a.State)
		}
		if a.Labels != "" {
			q.Set("labels", a.Labels)
		}
		kind := "/issues"
		if tool == "list_pull_requests" {
			kind = "/pulls"
		}
		var items []map[string]any
		if err := g.api(ctx, acct, "GET", repo+kind+"?"+q.Encode(), nil, &items); err != nil {
			return nil, err
		}
		out := make([]map[string]any, 0, len(items))
		for _, it := range items {
			if tool == "list_issues" && it["pull_request"] != nil {
				continue // the issues endpoint also returns PRs
			}
			out = append(out, map[string]any{"number": it["number"], "title": it["title"], "state": it["state"],
				"author": str(it, "user.login"), "url": it["html_url"], "updated_at": it["updated_at"]})
		}
		return out, nil
	case "get_issue":
		var issue map[string]any
		if err := g.api(ctx, acct, "GET", fmt.Sprintf("%s/issues/%d", repo, a.Number), nil, &issue); err != nil {
			return nil, err
		}
		var comments []map[string]any
		_ = g.api(ctx, acct, "GET", fmt.Sprintf("%s/issues/%d/comments?per_page=20", repo, a.Number), nil, &comments)
		var cs []map[string]string
		for _, c := range comments {
			cs = append(cs, map[string]string{"author": str(c, "user.login"), "body": truncateStr(str(c, "body"), 2000)})
		}
		return map[string]any{"number": issue["number"], "title": issue["title"], "state": issue["state"],
			"author": str(issue, "user.login"), "body": truncateStr(str(issue, "body"), 8000), "url": issue["html_url"],
			"is_pull_request": issue["pull_request"] != nil, "comments": cs}, nil
	case "create_issue":
		if strings.TrimSpace(a.Title) == "" {
			return nil, userErr("title is required")
		}
		body := map[string]any{"title": a.Title, "body": a.Body}
		if a.Labels != "" {
			body["labels"] = strings.Split(a.Labels, ",")
		}
		var issue map[string]any
		if err := g.api(ctx, acct, "POST", repo+"/issues", body, &issue); err != nil {
			return nil, err
		}
		return map[string]any{"number": issue["number"], "url": issue["html_url"]}, nil
	case "comment":
		if strings.TrimSpace(a.Body) == "" {
			return nil, userErr("body is required")
		}
		var c map[string]any
		if err := g.api(ctx, acct, "POST", fmt.Sprintf("%s/issues/%d/comments", repo, a.Number), map[string]string{"body": a.Body}, &c); err != nil {
			return nil, err
		}
		return map[string]any{"url": c["html_url"]}, nil
	case "close_issue":
		var issue map[string]any
		if err := g.api(ctx, acct, "PATCH", fmt.Sprintf("%s/issues/%d", repo, a.Number), map[string]string{"state": "closed"}, &issue); err != nil {
			return nil, err
		}
		return map[string]any{"number": issue["number"], "state": issue["state"]}, nil
	}
	return nil, userErr("unknown tool %q", tool)
}

func (g GitHub) Verify(ctx context.Context, acct Account) error {
	if acct.Credentials["token"] == "" {
		return userErr("a token is required")
	}
	var user map[string]any
	return g.api(ctx, acct, "GET", "/user", nil, &user)
}

func (GitHub) HandleWebhook(acct Account, r *http.Request, body []byte) ([]Signal, error) {
	sig := strings.TrimPrefix(r.Header.Get("X-Hub-Signature-256"), "sha256=")
	if !validHMACSHA256(acct.Credentials["webhook_secret"], body, sig) {
		return nil, ErrUnauthorized
	}
	event := r.Header.Get("X-GitHub-Event")
	var p map[string]any
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, userErr("invalid JSON")
	}
	f := map[string]string{"repo": str(p, "repository.full_name"), "action": str(p, "action"), "author": str(p, "sender.login")}
	switch event {
	case "ping":
		return nil, nil
	case "issues", "issue_comment":
		f["title"], f["number"], f["url"] = str(p, "issue.title"), str(p, "issue.number"), str(p, "issue.html_url")
		f["body"] = truncateStr(str(p, "issue.body"), 2000)
		if event == "issue_comment" {
			f["body"], f["url"] = truncateStr(str(p, "comment.body"), 2000), str(p, "comment.html_url")
		}
		var labels []string
		issue, _ := p["issue"].(map[string]any)
		if ls, ok := issue["labels"].([]any); ok {
			for _, l := range ls {
				if lm, ok := l.(map[string]any); ok {
					labels = append(labels, str(lm, "name"))
				}
			}
		}
		f["labels"] = strings.Join(labels, ",")
		f["assignee"] = str(p, "issue.assignee.login")
	case "pull_request":
		f["title"], f["number"], f["url"] = str(p, "pull_request.title"), str(p, "pull_request.number"), str(p, "pull_request.html_url")
		f["body"] = truncateStr(str(p, "pull_request.body"), 2000)
		f["merged"], f["base"], f["head"] = str(p, "pull_request.merged"), str(p, "pull_request.base.ref"), str(p, "pull_request.head.ref")
	case "push":
		f["ref"], f["url"], f["author"] = str(p, "ref"), str(p, "compare"), str(p, "pusher.name")
		if cs, ok := p["commits"].([]any); ok {
			var msgs []string
			for _, c := range cs {
				if cm, ok := c.(map[string]any); ok {
					msgs = append(msgs, strings.SplitN(str(cm, "message"), "\n", 2)[0])
				}
			}
			f["commits"] = truncateStr(strings.Join(msgs, "\n"), 2000)
		}
	case "workflow_run":
		f["name"], f["conclusion"] = str(p, "workflow_run.name"), str(p, "workflow_run.conclusion")
		f["branch"], f["url"] = str(p, "workflow_run.head_branch"), str(p, "workflow_run.html_url")
	default:
		return []Signal{{Type: "github." + event, Fields: f}}, nil
	}
	return []Signal{{Type: "github." + event, Fields: f}}, nil
}
