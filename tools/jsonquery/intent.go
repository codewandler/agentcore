package jsonquery

import "github.com/codewandler/agentsdk/action"

type queryAction struct{ action.Action }

func (a queryAction) DeclareIntent(ctx action.Ctx, input any) (action.Intent, error) {
	p, err := action.CastInput[QueryParams](input)
	if err != nil {
		return action.Intent{Action: "json_query", Class: "unknown", Opaque: true, Confidence: "low"}, nil
	}
	path := resolvePath(p.Path, workDirFromContext(ctx))
	return action.Intent{
		Action:     "json_query",
		Tool:       "json_query",
		Class:      "filesystem_read",
		ToolClass:  "filesystem_read",
		Confidence: "high",
		Operations: []action.IntentOperation{{
			Resource:  action.IntentResource{Category: "file", Value: path, Locality: classifyLocality(ctx, path)},
			Operation: "read",
			Certain:   true,
		}},
		Behaviors: []string{"filesystem_read"},
	}, nil
}

func classifyLocality(ctx action.Ctx, absPath string) string {
	workDir := workDirFromContext(ctx)
	if workDir == "" {
		return "unknown"
	}
	prefix := workDir
	if prefix[len(prefix)-1] != '/' {
		prefix += "/"
	}
	if absPath == workDir || len(absPath) >= len(prefix) && absPath[:len(prefix)] == prefix {
		return "workspace"
	}
	return "unknown"
}
