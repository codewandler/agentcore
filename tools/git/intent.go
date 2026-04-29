package git

import (
	"github.com/codewandler/agentsdk/tool"
)

func gitStatusIntent() tool.TypedToolOption[GitStatusParams] {
	return tool.WithDeclareIntent(func(ctx tool.Ctx, _ GitStatusParams) (tool.Intent, error) {
		return tool.Intent{
			Tool:       "git_status",
			ToolClass:  "repository_access",
			Confidence: "high",
			Operations: []tool.IntentOperation{{
				Resource:  tool.IntentResource{Category: "repo", Value: ctx.WorkDir(), Locality: "workspace"},
				Operation: "read",
				Certain:   true,
			}},
			Behaviors: []string{"filesystem_read"},
		}, nil
	})
}

func gitDiffIntent() tool.TypedToolOption[GitDiffParams] {
	return tool.WithDeclareIntent(func(ctx tool.Ctx, _ GitDiffParams) (tool.Intent, error) {
		return tool.Intent{
			Tool:       "git_diff",
			ToolClass:  "repository_access",
			Confidence: "high",
			Operations: []tool.IntentOperation{{
				Resource:  tool.IntentResource{Category: "repo", Value: ctx.WorkDir(), Locality: "workspace"},
				Operation: "read",
				Certain:   true,
			}},
			Behaviors: []string{"filesystem_read"},
		}, nil
	})
}
