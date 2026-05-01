package workflow

import (
	"fmt"
	"sync/atomic"

	gonanoid "github.com/matoous/go-nanoid/v2"
)

// RunID identifies one execution of a workflow definition.
type RunID string

var runSeq uint64

// NewRunID returns a workflow run identifier. Future durable harnesses may
// supply their own IDs through Executor.RunID or Executor.NewRunID.
func NewRunID() RunID {
	id, err := gonanoid.New(12)
	if err != nil {
		return RunID(fmt.Sprintf("run_%d", atomic.AddUint64(&runSeq, 1)))
	}
	return RunID("run_" + id)
}

// RunStatus is the materialized status of a workflow run.
type RunStatus string

const (
	RunRunning   RunStatus = "running"
	RunSucceeded RunStatus = "succeeded"
	RunFailed    RunStatus = "failed"
)

// StepStatus is the materialized status of a workflow step.
type StepStatus string

const (
	StepRunning      StepStatus = "running"
	StepSucceeded    StepStatus = "succeeded"
	StepFailedStatus StepStatus = "failed"
)

// RunState is the materialized state of one workflow execution.
type RunState struct {
	ID           RunID
	WorkflowName string
	Status       RunStatus
	Steps        map[string]StepState
	Output       any
	Error        string
}

// StepState is the materialized state of one workflow step.
type StepState struct {
	ID         string
	ActionName string
	Status     StepStatus
	Output     any
	Error      string
}

// Projector materializes workflow run state from concrete workflow event
// payloads. Non-workflow events are ignored so action-emitted events can share
// the same action.Result.Events slice.
type Projector struct{}

// Project materializes states for all workflow runs represented in events.
func (Projector) Project(events []any) (map[RunID]RunState, error) {
	states := map[RunID]RunState{}
	for _, event := range events {
		if err := applyEvent(states, event); err != nil {
			return nil, err
		}
	}
	return states, nil
}

// ProjectRun materializes the state for runID from events.
func (p Projector) ProjectRun(events []any, runID RunID) (RunState, bool, error) {
	states, err := p.Project(events)
	if err != nil {
		return RunState{}, false, err
	}
	state, ok := states[runID]
	return state, ok, nil
}

func applyEvent(states map[RunID]RunState, event any) error {
	switch e := event.(type) {
	case Started:
		state := stateFor(states, e.RunID)
		state.ID = e.RunID
		state.WorkflowName = e.WorkflowName
		state.Status = RunRunning
		states[e.RunID] = state
	case StepStarted:
		state := stateFor(states, e.RunID)
		step := state.Steps[e.StepID]
		step.ID = e.StepID
		step.ActionName = e.ActionName
		step.Status = StepRunning
		step.Error = ""
		state.Steps[e.StepID] = step
		states[e.RunID] = state
	case StepCompleted:
		state := stateFor(states, e.RunID)
		step := state.Steps[e.StepID]
		step.ID = e.StepID
		step.ActionName = e.ActionName
		step.Status = StepSucceeded
		step.Output = e.Data
		step.Error = ""
		state.Steps[e.StepID] = step
		states[e.RunID] = state
	case StepFailed:
		state := stateFor(states, e.RunID)
		step := state.Steps[e.StepID]
		step.ID = e.StepID
		step.ActionName = e.ActionName
		step.Status = StepFailedStatus
		step.Error = e.Error
		state.Steps[e.StepID] = step
		states[e.RunID] = state
	case Completed:
		state := stateFor(states, e.RunID)
		state.ID = e.RunID
		state.WorkflowName = e.WorkflowName
		state.Status = RunSucceeded
		state.Output = e.Data
		state.Error = ""
		states[e.RunID] = state
	case Failed:
		state := stateFor(states, e.RunID)
		state.ID = e.RunID
		state.WorkflowName = e.WorkflowName
		state.Status = RunFailed
		state.Error = e.Error
		states[e.RunID] = state
	case *Started, *StepStarted, *StepCompleted, *StepFailed, *Completed, *Failed:
		return fmt.Errorf("workflow: pointer events are not supported")
	default:
		return nil
	}
	return nil
}

func stateFor(states map[RunID]RunState, runID RunID) RunState {
	state := states[runID]
	if state.Steps == nil {
		state.Steps = map[string]StepState{}
	}
	return state
}
