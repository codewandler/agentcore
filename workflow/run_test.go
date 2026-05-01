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
		StepStarted{RunID: "run_1", WorkflowName: "shout", StepID: "upper", ActionName: "upper"},
		"ignored action event",
		StepCompleted{RunID: "run_1", WorkflowName: "shout", StepID: "upper", ActionName: "upper", Data: "HELLO"},
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
	require.Equal(t, "HELLO", state.Steps["upper"].Output)
}

func TestProjectorMaterializesFailedRunState(t *testing.T) {
	events := []any{
		Started{RunID: "run_1", WorkflowName: "failflow"},
		StepStarted{RunID: "run_1", WorkflowName: "failflow", StepID: "fail", ActionName: "fail"},
		StepFailed{RunID: "run_1", WorkflowName: "failflow", StepID: "fail", ActionName: "fail", Error: "boom"},
		Failed{RunID: "run_1", WorkflowName: "failflow", Error: "workflow failed: boom"},
	}

	state, ok, err := Projector{}.ProjectRun(events, "run_1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, RunFailed, state.Status)
	require.Equal(t, "workflow failed: boom", state.Error)
	require.Equal(t, StepFailedStatus, state.Steps["fail"].Status)
	require.Equal(t, "boom", state.Steps["fail"].Error)
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
