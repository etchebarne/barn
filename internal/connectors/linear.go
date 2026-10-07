package connectors

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// Linear: issues through the GraphQL API, and issue/comment events through webhooks.
type Linear struct{}

func (Linear) Name() string        { return "linear" }
func (Linear) DisplayName() string { return "Linear" }
func (Linear) Description() string {
	return "Search, create and update issues; get notified when issues change."
}
func (Linear) CredentialFields() []Field {
	return []Field{
		{Key: "api_key", Label: "API key", Secret: true, Help: "Starts with lin_api_."},
		{Key: "webhook_secret", Label: "Webhook signing secret", Secret: true, Optional: true, Events: true,
			Help: "Shown on the webhook you create in Linear."},
	}
}
func (Linear) ConfigFields() []Field { return nil }
func (Linear) SignalTypes() []SignalType {
	return []SignalType{
		{Type: "linear.issue", Description: "An issue was created, updated or removed",
			Fields: []string{"action", "identifier", "title", "state", "previous_state", "state_changed", "assignee", "team", "priority", "url"}},
		{Type: "linear.comment", Description: "Someone commented on an issue",
			Fields: []string{"action", "issue", "body", "author", "url"}},
	}
}

func (Linear) Tools(context.Context, Account) ([]Tool, error) {
	return []Tool{
		{Name: "search_issues", Description: "Find issues by text, team key, state name or assignee.",
			Parameters: params(map[string]string{"?query": "text in the title", "?team": "team key, e.g. ENG", "?state": "state name, e.g. In Progress", "?assignee": "assignee name or email"})},
		{Name: "get_issue", Description: "Get an issue by identifier (e.g. ENG-123) with its description and comments.",
			Parameters: params(map[string]string{"id": "identifier like ENG-123"})},
		{Name: "list_teams", Description: "List teams and their workflow states.", Parameters: params(map[string]string{})},
		{Name: "create_issue", Description: "Create an issue.", External: true,
			Title: "New issue", Verb: "Create issue", Body: "description",
			Parameters: params(map[string]string{"team": "team key, e.g. ENG", "title": "title", "?description": "Markdown description"})},
		{Name: "update_issue", Description: "Move an issue to another state and/or change its title.", External: true,
			Verb: "Update issue", Labels: map[string]string{"id": "Issue", "state": "Move to", "title": "New title"},
			Parameters: params(map[string]string{"id": "identifier like ENG-123", "?state": "state name, e.g. Done", "?title": "new title"})},
		{Name: "comment", Description: "Comment on an issue.", External: true,
			Title: "Comment", Verb: "Post comment", Body: "body", Labels: map[string]string{"id": "Issue"},
			Parameters: params(map[string]string{"id": "identifier like ENG-123", "body": "Markdown comment"})},
	}, nil
}

func (Linear) gql(ctx context.Context, acct Account, query string, vars map[string]any, out any) error {
	h := http.Header{}
	h.Set("Authorization", acct.Credentials["api_key"])
	var resp struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := apiRequest(ctx, "POST", baseURL(acct, "https://api.linear.app")+"/graphql", h,
		map[string]any{"query": query, "variables": vars}, &resp); err != nil {
		return err
	}
	if len(resp.Errors) > 0 {
		return userErr("Linear: %s", resp.Errors[0].Message)
	}
	return json.Unmarshal(resp.Data, out)
}

type linearIssue struct {
	ID          string `json:"id"`
	Identifier  string `json:"identifier"`
	Title       string `json:"title"`
	Description string `json:"description"`
	URL         string `json:"url"`
	Priority    int    `json:"priority"`
	State       struct {
		Name string `json:"name"`
	} `json:"state"`
	Assignee *struct {
		Name string `json:"name"`
	} `json:"assignee"`
	Team struct {
		ID  string `json:"id"`
		Key string `json:"key"`
	} `json:"team"`
}

const issueFields = `id identifier title url priority state { name } assignee { name } team { id key }`

func (l Linear) issue(ctx context.Context, acct Account, id string, withDetails bool) (linearIssue, error) {
	fields := issueFields
	if withDetails {
		fields += ` description`
	}
	var out struct {
		Issue *linearIssue `json:"issue"`
	}
	if err := l.gql(ctx, acct, `query($id: String!) { issue(id: $id) { `+fields+` } }`, map[string]any{"id": id}, &out); err != nil {
		return linearIssue{}, err
	}
	if out.Issue == nil {
		return linearIssue{}, userErr("no issue %q", id)
	}
	return *out.Issue, nil
}

func summarizeIssue(i linearIssue) map[string]any {
	out := map[string]any{"id": i.Identifier, "title": i.Title, "state": i.State.Name, "team": i.Team.Key, "url": i.URL, "priority": i.Priority}
	if i.Assignee != nil {
		out["assignee"] = i.Assignee.Name
	}
	return out
}

func (l Linear) Call(ctx context.Context, acct Account, tool string, args json.RawMessage) (any, error) {
	var a struct {
		ID, Query, Team, State, Assignee, Title, Description, Body string
	}
	if err := decodeArgs(args, &a); err != nil {
		return nil, err
	}
	switch tool {
	case "search_issues":
		filter := map[string]any{}
		if a.Query != "" {
			filter["title"] = map[string]any{"containsIgnoreCase": a.Query}
		}
		if a.Team != "" {
			filter["team"] = map[string]any{"key": map[string]any{"eqIgnoreCase": a.Team}}
		}
		if a.State != "" {
			filter["state"] = map[string]any{"name": map[string]any{"eqIgnoreCase": a.State}}
		}
		if a.Assignee != "" {
			filter["assignee"] = map[string]any{"or": []any{
				map[string]any{"name": map[string]any{"containsIgnoreCase": a.Assignee}},
				map[string]any{"email": map[string]any{"eqIgnoreCase": a.Assignee}},
			}}
		}
		var out struct {
			Issues struct {
				Nodes []linearIssue `json:"nodes"`
			} `json:"issues"`
		}
		if err := l.gql(ctx, acct, `query($filter: IssueFilter) { issues(first: 25, filter: $filter, orderBy: updatedAt) { nodes { `+issueFields+` } } }`,
			map[string]any{"filter": filter}, &out); err != nil {
			return nil, err
		}
		list := make([]map[string]any, 0, len(out.Issues.Nodes))
		for _, i := range out.Issues.Nodes {
			list = append(list, summarizeIssue(i))
		}
		return list, nil
	case "get_issue":
		i, err := l.issue(ctx, acct, a.ID, true)
		if err != nil {
			return nil, err
		}
		var comments struct {
			Issue struct {
				Comments struct {
					Nodes []struct {
						Body string `json:"body"`
						User *struct {
							Name string `json:"name"`
						} `json:"user"`
					} `json:"nodes"`
				} `json:"comments"`
			} `json:"issue"`
		}
		_ = l.gql(ctx, acct, `query($id: String!) { issue(id: $id) { comments(first: 20) { nodes { body user { name } } } } }`, map[string]any{"id": a.ID}, &comments)
		out := summarizeIssue(i)
		out["description"] = truncateStr(i.Description, 8000)
		var cs []map[string]string
		for _, c := range comments.Issue.Comments.Nodes {
			author := ""
			if c.User != nil {
				author = c.User.Name
			}
			cs = append(cs, map[string]string{"author": author, "body": truncateStr(c.Body, 2000)})
		}
		out["comments"] = cs
		return out, nil
	case "list_teams":
		var out struct {
			Teams struct {
				Nodes []struct {
					Key    string `json:"key"`
					Name   string `json:"name"`
					States struct {
						Nodes []struct {
							Name string `json:"name"`
							Type string `json:"type"`
						} `json:"nodes"`
					} `json:"states"`
				} `json:"nodes"`
			} `json:"teams"`
		}
		if err := l.gql(ctx, acct, `{ teams { nodes { key name states { nodes { name type } } } } }`, nil, &out); err != nil {
			return nil, err
		}
		return out.Teams.Nodes, nil
	case "create_issue":
		if strings.TrimSpace(a.Title) == "" || a.Team == "" {
			return nil, userErr("team and title are required")
		}
		var teams struct {
			Teams struct {
				Nodes []struct {
					ID string `json:"id"`
				} `json:"nodes"`
			} `json:"teams"`
		}
		if err := l.gql(ctx, acct, `query($key: String!) { teams(filter: { key: { eqIgnoreCase: $key } }) { nodes { id } } }`,
			map[string]any{"key": a.Team}, &teams); err != nil {
			return nil, err
		}
		if len(teams.Teams.Nodes) == 0 {
			return nil, userErr("no team with key %q (see list_teams)", a.Team)
		}
		var out struct {
			IssueCreate struct {
				Issue linearIssue `json:"issue"`
			} `json:"issueCreate"`
		}
		if err := l.gql(ctx, acct, `mutation($input: IssueCreateInput!) { issueCreate(input: $input) { issue { `+issueFields+` } } }`,
			map[string]any{"input": map[string]any{"teamId": teams.Teams.Nodes[0].ID, "title": a.Title, "description": a.Description}}, &out); err != nil {
			return nil, err
		}
		return summarizeIssue(out.IssueCreate.Issue), nil
	case "update_issue":
		i, err := l.issue(ctx, acct, a.ID, false)
		if err != nil {
			return nil, err
		}
		input := map[string]any{}
		if a.Title != "" {
			input["title"] = a.Title
		}
		if a.State != "" {
			var states struct {
				WorkflowStates struct {
					Nodes []struct {
						ID string `json:"id"`
					} `json:"nodes"`
				} `json:"workflowStates"`
			}
			if err := l.gql(ctx, acct, `query($team: ID!, $name: String!) { workflowStates(filter: { team: { id: { eq: $team } }, name: { eqIgnoreCase: $name } }) { nodes { id } } }`,
				map[string]any{"team": i.Team.ID, "name": a.State}, &states); err != nil {
				return nil, err
			}
			if len(states.WorkflowStates.Nodes) == 0 {
				return nil, userErr("team %s has no state %q (see list_teams)", i.Team.Key, a.State)
			}
			input["stateId"] = states.WorkflowStates.Nodes[0].ID
		}
		if len(input) == 0 {
			return nil, userErr("nothing to change: give state and/or title")
		}
		var out struct {
			IssueUpdate struct {
				Issue linearIssue `json:"issue"`
			} `json:"issueUpdate"`
		}
		if err := l.gql(ctx, acct, `mutation($id: String!, $input: IssueUpdateInput!) { issueUpdate(id: $id, input: $input) { issue { `+issueFields+` } } }`,
			map[string]any{"id": i.ID, "input": input}, &out); err != nil {
			return nil, err
		}
		return summarizeIssue(out.IssueUpdate.Issue), nil
	case "comment":
		if strings.TrimSpace(a.Body) == "" {
			return nil, userErr("body is required")
		}
		i, err := l.issue(ctx, acct, a.ID, false)
		if err != nil {
			return nil, err
		}
		var out struct {
			CommentCreate struct {
				Comment struct {
					URL string `json:"url"`
				} `json:"comment"`
			} `json:"commentCreate"`
		}
		if err := l.gql(ctx, acct, `mutation($input: CommentCreateInput!) { commentCreate(input: $input) { comment { url } } }`,
			map[string]any{"input": map[string]any{"issueId": i.ID, "body": a.Body}}, &out); err != nil {
			return nil, err
		}
		return map[string]string{"url": out.CommentCreate.Comment.URL}, nil
	}
	return nil, userErr("unknown tool %q", tool)
}

func (l Linear) Verify(ctx context.Context, acct Account) error {
	var out struct {
		Viewer struct {
			ID string `json:"id"`
		} `json:"viewer"`
	}
	return l.gql(ctx, acct, `{ viewer { id } }`, nil, &out)
}

func (Linear) HandleWebhook(acct Account, r *http.Request, body []byte) ([]Signal, error) {
	if !validHMACSHA256(acct.Credentials["webhook_secret"], body, r.Header.Get("Linear-Signature")) {
		return nil, ErrUnauthorized
	}
	var p map[string]any
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, userErr("invalid JSON")
	}
	switch str(p, "type") {
	case "Issue":
		f := map[string]string{
			"action": str(p, "action"), "identifier": str(p, "data.identifier"), "title": str(p, "data.title"),
			"state": str(p, "data.state.name"), "assignee": str(p, "data.assignee.name"), "team": str(p, "data.team.key"),
			"priority": str(p, "data.priorityLabel"), "url": str(p, "url"),
		}
		changed := str(p, "updatedFrom.stateId") != ""
		f["state_changed"] = fmt.Sprint(changed)
		if changed {
			f["previous_state"] = str(p, "updatedFrom.state.name")
		}
		return []Signal{{Type: "linear.issue", Fields: f}}, nil
	case "Comment":
		return []Signal{{Type: "linear.comment", Fields: map[string]string{
			"action": str(p, "action"), "issue": str(p, "data.issue.identifier"), "body": truncateStr(str(p, "data.body"), 2000),
			"author": str(p, "data.user.name"), "url": str(p, "url"),
		}}}, nil
	}
	return nil, nil
}
