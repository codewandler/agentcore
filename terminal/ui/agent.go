package ui

import (
	"io"
	"strconv"

	"github.com/codewandler/agentsdk/agent"
	"github.com/codewandler/agentsdk/runner"
)

// AgentEventHandlerFactory adapts terminal event rendering to agent turn events.
// The agent remains responsible for recording usage and route state; this
// factory only renders events at the terminal boundary.
func AgentEventHandlerFactory(out io.Writer) func(*agent.Instance, int) runner.EventHandler {
	return func(inst *agent.Instance, turnID int) runner.EventHandler {
		if out == nil {
			return nil
		}
		spec := inst.Spec()
		return NewEventDisplay(out,
			WithTurnID(strconv.Itoa(turnID)),
			WithSessionID(inst.SessionID()),
			WithFallbackModel(spec.Inference.Model),
		).Handler()
	}
}
