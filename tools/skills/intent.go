package skills

import "github.com/codewandler/agentsdk/tool"

func skillIntent() tool.TypedToolOption[actionParams] {
	return tool.WithDeclareIntent(func(_ tool.Ctx, _ actionParams) (tool.Intent, error) {
		return tool.Intent{
			Tool:       "skill",
			ToolClass:  "agent_control",
			Confidence: "high",
			Behaviors:  []string{"agent_control"},
		}, nil
	})
}
