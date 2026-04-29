package toolmgmt

import "github.com/codewandler/agentsdk/tool"

func toolsListIntent() tool.TypedToolOption[ToolListParams] {
	return tool.WithDeclareIntent(func(_ tool.Ctx, _ ToolListParams) (tool.Intent, error) {
		return tool.Intent{
			Tool:       "tools_list",
			ToolClass:  "agent_control",
			Confidence: "high",
			Behaviors:  []string{"agent_control"},
		}, nil
	})
}

func toolsActivateIntent() tool.TypedToolOption[ToolActivateParams] {
	return tool.WithDeclareIntent(func(_ tool.Ctx, _ ToolActivateParams) (tool.Intent, error) {
		return tool.Intent{
			Tool:       "tools_activate",
			ToolClass:  "agent_control",
			Confidence: "high",
			Behaviors:  []string{"agent_control"},
		}, nil
	})
}

func toolsDeactivateIntent() tool.TypedToolOption[ToolDeactivateParams] {
	return tool.WithDeclareIntent(func(_ tool.Ctx, _ ToolDeactivateParams) (tool.Intent, error) {
		return tool.Intent{
			Tool:       "tools_deactivate",
			ToolClass:  "agent_control",
			Confidence: "high",
			Behaviors:  []string{"agent_control"},
		}, nil
	})
}
