package todo

import "github.com/codewandler/agentsdk/tool"

func todoIntent() tool.TypedToolOption[Params] {
	return tool.WithDeclareIntent(func(_ tool.Ctx, p Params) (tool.Intent, error) {
		op := "read"
		behavior := "filesystem_read"
		switch p.Action {
		case "create", "update", "delete":
			op = "write"
			behavior = "filesystem_write"
		}
		return tool.Intent{
			Tool:       "todo",
			ToolClass:  "agent_control",
			Confidence: "high",
			Operations: []tool.IntentOperation{{
				Resource:  tool.IntentResource{Category: "config", Value: "todo_list", Locality: "workspace"},
				Operation: op,
				Certain:   true,
			}},
			Behaviors: []string{behavior},
		}, nil
	})
}
