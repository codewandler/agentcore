package command

import (
	"strings"

	"github.com/codewandler/agentsdk/tool"
)

type runParams struct {
	Command string `json:"command" jsonschema:"description=Command line to execute, including the leading slash or command name."`
}

// Tool returns a tool that exposes agent-callable commands from reg.
func Tool(reg *Registry) tool.Tool {
	return tool.New("command_run", "Run an app command explicitly exposed to agents.", func(ctx tool.Ctx, p runParams) (tool.Result, error) {
		if reg == nil {
			return tool.Error("no command registry configured"), nil
		}
		line := strings.TrimSpace(p.Command)
		if line == "" {
			return tool.Error("command is required"), nil
		}
		result, err := reg.ExecuteAgent(ctx, line)
		if err != nil {
			return tool.Errorf("%v", err), nil
		}
		switch result.Kind {
		case ResultHandled:
			return tool.Text("command handled"), nil
		case ResultText:
			return tool.Text(result.Text), nil
		case ResultAgentTurn:
			return tool.Text(result.Input), nil
		default:
			return tool.Error("command result cannot be applied from agent context"), nil
		}
	})
}
