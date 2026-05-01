package workflow

import (
	"context"
	"testing"

	"github.com/codewandler/agentsdk/action"
	"github.com/stretchr/testify/require"
)

func TestProjectorMaterializesSuccessfulRunState(t *testing.T) {
	events := []any{
		Started{RunID: "run_1", WorkflowName: "shout"},
		StepStarted{RunID: "run_1", WorkflowName: "shout", StepID: "upper", ActionName: "upper", Attempt: 1},
		"ignored action event",
		StepCompleted{RunID: "run_1", WorkflowName: "shout", StepID: "upper", ActionName: "upper", Attempt: 1, Data: "HELLO"},
		Completed{RunID: "run_1", WorkflowName: "shout", Data: "HELLO"},
	}

	state, ok, err := Projector{}.ProjectRun(events, "run_1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, RunID("run_1"), state.ID)
	require.Equal(t, "shout", state.WorkflowName)
	require.Equal(t, RunSucceeded, state.Status)
	require.Equal(t, "HELLO", state.Output)
	require.Equal(t, StepSucceeded, state.Steps["upper"].Status)
	require.Equal(t, 1, state.Steps["upper"].Attempt)
	require.Equal(t, "HELLO", state.Steps["upper"].Output)
	require.Equal(t, []AttemptState{{Attempt: 1, Status: StepSucceeded, Output: "HELLO"}}, state.Steps["upper"].Attempts)
}

func TestProjectorMaterializesFailedRunState(t *testing.T) {
	events := []any{
		Started{RunID: "run_1", WorkflowName: "failflow"},
		StepStarted{RunID: "run_1", WorkflowName: "failflow", StepID: "fail", ActionName: "fail", Attempt: 1},
		StepFailed{RunID: "run_1", WorkflowName: "failflow", StepID: "fail", ActionName: "fail", Attempt: 1, Error: "boom"},
		Failed{RunID: "run_1", WorkflowName: "failflow", Error: "workflow failed: boom"},
	}

	state, ok, err := Projector{}.ProjectRun(events, "run_1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, RunFailed, state.Status)
	require.Equal(t, "workflow failed: boom", state.Error)
	require.Equal(t, StepFailedStatus, state.Steps["fail"].Status)
	require.Equal(t, 1, state.Steps["fail"].Attempt)
	require.Equal(t, "boom", state.Steps["fail"].Error)
	require.Equal(t, []AttemptState{{Attempt: 1, Status: StepFailedStatus, Error: "boom"}}, state.Steps["fail"].Attempts)
}

func TestProjectorKeepsRunsSeparate(t *testing.T) {
	events := []any{
		Started{RunID: "run_1", WorkflowName: "echo"},
		Completed{RunID: "run_1", WorkflowName: "echo", Data: "one"},
		Started{RunID: "run_2", WorkflowName: "echo"},
		Completed{RunID: "run_2", WorkflowName: "echo", Data: "two"},
	}

	states, err := Projector{}.Project(events)
	require.NoError(t, err)
	require.Equal(t, "one", states["run_1"].Output)
	require.Equal(t, "two", states["run_2"].Output)
}

func TestProjectorRejectsPointerWorkflowEvents(t *testing.T) {
	_, err := Projector{}.Project([]any{&Started{RunID: "run_1", WorkflowName: "echo"}})
	require.Error(t, err)
}

func TestProjectorTracksMultipleAttempts(t *testing.T) {
	events := []any{
		Started{RunID: "run_1", WorkflowName: "retryflow"},
		StepStarted{RunID: "run_1", WorkflowName: "retryflow", StepID: "fetch", ActionName: "fetch", Attempt: 1},
		StepFailed{RunID: "run_1", WorkflowName: "retryflow", StepID: "fetch", ActionName: "fetch", Attempt: 1, Error: "temporary"},
		StepStarted{RunID: "run_1", WorkflowName: "retryflow", StepID: "fetch", ActionName: "fetch", Attempt: 2},
		StepCompleted{RunID: "run_1", WorkflowName: "retryflow", StepID: "fetch", ActionName: "fetch", Attempt: 2, Data: "ok"},
		Completed{RunID: "run_1", WorkflowName: "retryflow", Data: "ok"},
	}

	state, ok, err := Projector{}.ProjectRun(events, "run_1")
	require.NoError(t, err)
	require.True(t, ok)
	step := state.Steps["fetch"]
	require.Equal(t, StepSucceeded, step.Status)
	require.Equal(t, 2, step.Attempt)
	require.Equal(t, []AttemptState{
		{Attempt: 1, Status: StepFailedStatus, Error: "temporary"},
		{Attempt: 2, Status: StepSucceeded, Output: "ok"},
	}, step.Attempts)
}
func TestExecutorResultCarriesRunIDForProjection(t *testing.T) {
	reg := action.NewRegistry()
	require.NoError(t, reg.Register(action.New(action.Spec{Name: "echo"}, func(action.Ctx, any) action.Result {
		return action.Result{Data: "ok"}
	})))
	result := Executor{Resolver: RegistryResolver{Registry: reg}, RunID: "run_1"}.Execute(context.Background(), Definition{Name: "echo", Steps: []Step{{ID: "echo", Action: ActionRef{Name: "echo"}}}}, nil)
	require.NoError(t, result.Error)
	wfResult := result.Data.(Result)
	require.Equal(t, RunID("run_1"), wfResult.RunID)
	state, ok, err := Projector{}.ProjectRun(result.Events, wfResult.RunID)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, RunSucceeded, state.Status)
}
