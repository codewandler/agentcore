package notify

import "github.com/codewandler/agentsdk/tool"

func notifyIntent() tool.TypedToolOption[NotifyParams] {
	return tool.WithDeclareIntent(func(_ tool.Ctx, _ NotifyParams) (tool.Intent, error) {
		return tool.Intent{
			Tool:       "notify_send",
			ToolClass:  "agent_control",
			Confidence: "high",
			Behaviors:  []string{"agent_control"},
		}, nil
	})
}
