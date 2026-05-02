package harness

import (
	"encoding/json"
	"fmt"

	"github.com/codewandler/agentsdk/action"
	"github.com/codewandler/agentsdk/command"
)

const CommandActionName = "command.execute"

// CommandAction returns an action adapter for trusted command envelope execution.
// It is intended for SDK/API/workflow callers. Agent-facing tool adapters should
// use ExecuteAgentCommandEnvelope instead.
func (s *Session) CommandAction() action.Action {
	return action.New(action.Spec{
		Name:        CommandActionName,
		Description: "Execute a command envelope through the active harness session.",
		Input:       action.TypeOf[CommandEnvelope](),
		Output:      action.TypeOf[command.Result](),
	}, func(ctx action.Ctx, input any) action.Result {
		envelope, err := commandEnvelopeFromActionInput(input)
		if err != nil {
			return action.Result{Error: err}
		}
		result, err := s.ExecuteCommandEnvelope(ctx, envelope)
		if err != nil {
			return action.Result{Error: err}
		}
		return action.Result{Data: result}
	})
}

func commandEnvelopeFromActionInput(input any) (CommandEnvelope, error) {
	switch v := input.(type) {
	case nil:
		return CommandEnvelope{}, nil
	case CommandEnvelope:
		return v, nil
	case *CommandEnvelope:
		if v == nil {
			return CommandEnvelope{}, nil
		}
		return *v, nil
	default:
		data, err := json.Marshal(input)
		if err != nil {
			return CommandEnvelope{}, fmt.Errorf("harness: encode command envelope action input: %w", err)
		}
		var envelope CommandEnvelope
		if err := json.Unmarshal(data, &envelope); err != nil {
			return CommandEnvelope{}, fmt.Errorf("harness: decode command envelope action input: %w", err)
		}
		return envelope, nil
	}
}
