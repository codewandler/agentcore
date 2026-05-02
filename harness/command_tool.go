package harness

import (
	"context"
	"strings"

	"github.com/codewandler/agentsdk/command"
)

// CommandToolInput is the generic envelope accepted by the single command
// dispatcher tool. Exact per-command input schemas are provided through the
// agent-callable command catalog and enforced by command tree execution.
type CommandToolInput struct {
	Path  []string       `json:"path"`
	Input map[string]any `json:"input,omitempty"`
}

// CommandToolSchema returns the generic command dispatcher tool input schema.
func CommandToolSchema() command.JSONSchema {
	return command.JSONSchema{
		Type:        "object",
		Description: "Execute an agent-callable command from the provided command catalog.",
		Properties: map[string]command.JSONSchema{
			"path": {
				Type:        "array",
				Description: "Command path from the agent-callable command catalog, for example [\"workflow\", \"show\"].",
				Items:       &command.JSONSchema{Type: "string"},
			},
			"input": {
				Type:        "object",
				Description: "Structured command input matching the selected command descriptor.",
			},
		},
		Required: []string{"path"},
	}
}

// AgentCommandCatalog returns the commands available through the command tool.
func (s *Session) AgentCommandCatalog() []CommandCatalogEntry {
	return s.CommandCatalog(CommandCatalogAgentCallable())
}

// ExecuteCommandTool executes one agent-callable command through the generic
// command dispatcher envelope.
func (s *Session) ExecuteCommandTool(ctx context.Context, input CommandToolInput) (command.Result, error) {
	path := commandPath(input.Path)
	if len(path) == 0 {
		return command.Result{}, command.ValidationError{Code: command.ValidationInvalidSpec, Message: "harness: command tool path is required"}
	}
	if !s.agentCallableCommandPath(path) {
		return command.Result{}, command.ErrNotCallable{Name: strings.Join(path, " "), Caller: "agent"}
	}
	return s.ExecuteCommand(ctx, path, input.Input)
}

func (s *Session) agentCallableCommandPath(path []string) bool {
	for _, entry := range s.AgentCommandCatalog() {
		if sameCommandPath(entry.Descriptor.Path, path) {
			return true
		}
	}
	return false
}

func sameCommandPath(a []string, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
