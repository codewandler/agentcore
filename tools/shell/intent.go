package shell

import (
	"strings"

	"github.com/codewandler/agentsdk/tool"
	"github.com/codewandler/cmdrisk"
)

// bashIntent returns a DeclareIntent option for the bash tool.
// If analyzer is non-nil, it uses cmdrisk for full command analysis.
// Otherwise, it returns an opaque intent.
func bashIntent(analyzer *cmdrisk.Analyzer) tool.TypedToolOption[BashParams] {
	return tool.WithDeclareIntent(func(ctx tool.Ctx, p BashParams) (tool.Intent, error) {
		commands := []string(p.Cmd)
		if len(commands) == 0 {
			return tool.Intent{
				Tool:       "bash",
				ToolClass:  "command_execution",
				Opaque:     true,
				Confidence: "low",
				Behaviors:  []string{"command_execution"},
			}, nil
		}

		if analyzer == nil {
			return tool.Intent{
				Tool:       "bash",
				ToolClass:  "command_execution",
				Opaque:     true,
				Confidence: "low",
				Behaviors:  []string{"command_execution"},
			}, nil
		}

		// Use cmdrisk for full analysis.
		command := strings.Join(commands, "; ")
		workDir := p.Workdir
		if workDir == "" {
			workDir = ctx.WorkDir()
		}

		assessment, err := analyzer.Assess(ctx, cmdrisk.Request{
			Command: command,
			Context: cmdrisk.Context{
				Environment: cmdrisk.EnvironmentDeveloperWorkstation,
				Interactive: true,
				Asset: cmdrisk.AssetContext{
					WorkspacePathPrefixes: []string{workDir},
				},
				Trust: cmdrisk.TrustContext{
					CommandOrigin: cmdrisk.CommandOriginMachineGenerated,
				},
			},
		})
		if err != nil {
			return tool.Intent{
				Tool:       "bash",
				ToolClass:  "command_execution",
				Opaque:     true,
				Confidence: "low",
				Behaviors:  []string{"command_execution"},
			}, nil
		}

		// Map cmdrisk targets → IntentOperations.
		ops := make([]tool.IntentOperation, 0, len(assessment.Targets))
		for _, target := range assessment.Targets {
			ops = append(ops, tool.IntentOperation{
				Resource: tool.IntentResource{
					Category: target.Category,
					Value:    target.Value,
					Locality: target.Locality,
				},
				Operation: target.Role,
				Certain:   target.Certain,
			})
		}

		return tool.Intent{
			Tool:       "bash",
			ToolClass:  "command_execution",
			Operations: ops,
			Behaviors:  assessment.Behaviors,
			Confidence: string(assessment.Confidence),
			Extra:      &assessment, // CmdRiskAssessor reuses this — no double work
		}, nil
	})
}
