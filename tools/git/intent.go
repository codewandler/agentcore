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

func gitAddIntent() tool.TypedToolOption[GitAddParams] {
	return tool.WithDeclareIntent(func(ctx tool.Ctx, _ GitAddParams) (tool.Intent, error) {
		return tool.Intent{
			Tool:       "git_add",
			ToolClass:  "repository_modify",
			Confidence: "high",
			Operations: []tool.IntentOperation{{
				Resource:  tool.IntentResource{Category: "repo", Value: ctx.WorkDir(), Locality: "workspace"},
				Operation: "write",
				Certain:   true,
			}},
			Behaviors: []string{"filesystem_read", "repo_index_modify"},
		}, nil
	})
}

func gitCommitIntent() tool.TypedToolOption[GitCommitParams] {
	return tool.WithDeclareIntent(func(ctx tool.Ctx, _ GitCommitParams) (tool.Intent, error) {
		return tool.Intent{
			Tool:       "git_commit",
			ToolClass:  "repository_modify",
			Confidence: "high",
			Operations: []tool.IntentOperation{{
				Resource:  tool.IntentResource{Category: "repo", Value: ctx.WorkDir(), Locality: "workspace"},
				Operation: "write",
				Certain:   true,
			}},
			Behaviors: []string{"filesystem_read", "repo_history_modify"},
		}, nil
	})
}
