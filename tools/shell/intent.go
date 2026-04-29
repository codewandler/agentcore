package shell

import (
	"github.com/codewandler/agentsdk/tool"
)

// bashIntent returns an opaque intent for bash. Full intent extraction
// via cmdrisk is deferred to Phase 5.
func bashIntent() tool.TypedToolOption[BashParams] {
	return tool.WithDeclareIntent(func(_ tool.Ctx, _ BashParams) (tool.Intent, error) {
		return tool.Intent{
			Tool:       "bash",
			ToolClass:  "command_execution",
			Opaque:     true,
			Confidence: "low",
			Behaviors:  []string{"command_execution"},
		}, nil
	})
}
