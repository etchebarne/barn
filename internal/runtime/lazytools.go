package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/etchebarne/openbot/internal/model"
	"github.com/etchebarne/openbot/internal/store"
)

// Apps with many tools (an MCP server can bring dozens) would put all their definitions in every
// request. Past a size, the agent gets a one-line catalog in its prompt and two small tools
// instead: app_tool_info for a tool's parameters, app_tool_call to call it. A bridged call is
// unwrapped into the real tool call before it runs, so approvals, reports and limits apply as if
// the agent had called the tool directly.

const (
	toolAppToolInfo = "app_tool_info"
	toolAppToolCall = "app_tool_call"

	lazyToolCount = 15     // more app tools than this…
	lazyToolChars = 16_000 // …or definitions bigger than this, and they load on demand
	catalogChars  = 16_000 // the catalog shrinks to names past this
)

var (
	appToolInfoTool = function(toolAppToolInfo,
		"Get the full description and parameters of app tools from the catalog in your prompt "+
			"(Your app tools), before calling them with app_tool_call.",
		`{
			"type": "object",
			"properties": {"names": {"type": "array", "items": {"type": "string"}, "description": "Tool names, e.g. [\"linear__list_issues\"]."}},
			"required": ["names"],
			"additionalProperties": false
		}`)
	appToolCallTool = function(toolAppToolCall,
		"Call an app tool from the catalog in your prompt (Your app tools). Check its parameters "+
			"with app_tool_info first. Tools that act on the user's behalf need approval, as usual.",
		`{
			"type": "object",
			"properties": {
				"name": {"type": "string", "description": "The tool, e.g. linear__create_issue."},
				"arguments": {"type": "object", "description": "Its arguments."}
			},
			"required": ["name", "arguments"],
			"additionalProperties": false
		}`)
)

// lazyApps reports whether the agent's app tools are big enough to load on demand.
func lazyApps(tools []model.Tool) bool {
	if len(tools) > lazyToolCount {
		return true
	}
	size := 0
	for _, t := range tools {
		b, _ := json.Marshal(t)
		size += len(b)
	}
	return size > lazyToolChars
}

// unwrapAppCall turns app_tool_call into the call it stands for (same id, so the result still
// answers the model's call). Other calls come back as they are.
func unwrapAppCall(call model.ToolCall) model.ToolCall {
	if call.Function.Name != toolAppToolCall {
		return call
	}
	var a struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if json.Unmarshal([]byte(call.Function.Arguments), &a) != nil || !strings.Contains(a.Name, "__") {
		return call // runTool explains what's wrong
	}
	args := string(a.Arguments)
	if args == "" || args == "null" {
		args = "{}"
	}
	call.Function.Name, call.Function.Arguments = a.Name, args
	return call
}

// appToolInfo describes app tools by name.
func (l *loop) appToolInfo(ctx context.Context, agent store.Agent, raw []byte) (string, bool) {
	var a struct {
		Names []string `json:"names"`
	}
	if err := json.Unmarshal(raw, &a); err != nil || len(a.Names) == 0 {
		return toolError("give the names of the tools, e.g. {\"names\": [\"linear__list_issues\"]}"), false
	}
	var out []map[string]any
	var missing []string
	for _, name := range a.Names {
		t, ok := l.connectorTool(ctx, agent, name)
		if !ok {
			missing = append(missing, name)
			continue
		}
		out = append(out, map[string]any{"name": t.Name, "description": t.Tool.Description,
			"parameters": json.RawMessage(t.Tool.Parameters), "acts_on_users_behalf": t.Tool.External})
	}
	res := map[string]any{"tools": out}
	if len(missing) > 0 {
		res["unknown"] = missing
	}
	return toolOK(res), true
}

// writeAppCatalog lists the agent's app tools in its prompt when they load on demand.
func (l *loop) writeAppCatalog(ctx context.Context, b *strings.Builder, agent store.Agent) {
	tools := l.connectorTools(ctx, agent)
	if !lazyApps(asModelTools(ctx, l.m, tools)) {
		return
	}
	b.WriteString("# Your app tools\n")
	b.WriteString("Your connected apps have many tools, so they load on demand: check a tool's parameters with " +
		"app_tool_info, then call it with app_tool_call.\n")
	var full strings.Builder
	for _, t := range tools {
		fmt.Fprintf(&full, "- %s: %s\n", t.Name, truncate(firstLine(t.Tool.Description), 120))
	}
	if full.Len() <= catalogChars {
		b.WriteString(full.String())
	} else {
		byApp := map[string][]string{}
		var apps []string
		for _, t := range tools {
			if _, ok := byApp[t.AccountID]; !ok {
				apps = append(apps, t.AccountID)
			}
			byApp[t.AccountID] = append(byApp[t.AccountID], t.Name)
		}
		for _, id := range apps {
			fmt.Fprintf(b, "- %s\n", strings.Join(byApp[id], ", "))
		}
	}
	b.WriteString("\n")
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return line
}
