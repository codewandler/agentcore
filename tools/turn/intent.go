package turn

import "github.com/codewandler/agentsdk/tool"

func turnDoneIntent() tool.TypedToolOption[TurnDoneParams] {
	return tool.WithDeclareIntent(func(_ tool.Ctx, _ TurnDoneParams) (tool.Intent, error) {
		return tool.Intent{
			Tool:       "turn_done",
			ToolClass:  "agent_control",
			Confidence: "high",
			Behaviors:  []string{"agent_control"},
		}, nil
	})
}
