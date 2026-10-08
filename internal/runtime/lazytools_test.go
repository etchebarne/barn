package runtime

import (
	"fmt"
	"strings"
	"testing"

	"github.com/etchebarne/openbot/internal/model"
)

func TestLazyAppTools(t *testing.T) {
	tool := func(name, desc string) model.Tool {
		return model.Tool{Type: "function", Function: model.FunctionSpec{Name: name, Description: desc, Parameters: []byte(`{"type":"object"}`)}}
	}
	var few, many []model.Tool
	for i := range 5 {
		few = append(few, tool(fmt.Sprintf("app__t%d", i), "Does a thing."))
	}
	for i := range 20 {
		many = append(many, tool(fmt.Sprintf("app__t%d", i), "Does a thing."))
	}
	if lazyApps(few) || !lazyApps(many) {
		t.Fatal("more than 15 tools should load on demand, a few shouldn't")
	}
	if !lazyApps([]model.Tool{tool("app__big", strings.Repeat("Long description. ", 1000))}) {
		t.Fatal("very large definitions should load on demand")
	}

	call := model.ToolCall{ID: "c1", Function: model.FunctionCall{Name: toolAppToolCall,
		Arguments: `{"name":"linear__create_issue","arguments":{"title":"Bug"}}`}}
	got := unwrapAppCall(call)
	if got.ID != "c1" || got.Function.Name != "linear__create_issue" || got.Function.Arguments != `{"title":"Bug"}` {
		t.Fatalf("unwrapped: %+v", got)
	}
	if got := unwrapAppCall(model.ToolCall{Function: model.FunctionCall{Name: toolAppToolCall, Arguments: `{"name":"send_message"}`}}); got.Function.Name != toolAppToolCall {
		t.Fatal("only app tools (account__tool) can be called through the bridge")
	}
	if got := unwrapAppCall(model.ToolCall{Function: model.FunctionCall{Name: toolSendMessage}}); got.Function.Name != toolSendMessage {
		t.Fatal("other calls pass through")
	}
}
