package workflow

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMemoryRunStoreEventsAreIsolatedFromSliceMutation(t *testing.T) {
	store := NewMemoryRunStore()
	events := []any{
		Started{RunID: "run_1", WorkflowName: "echo"},
		Completed{RunID: "run_1", WorkflowName: "echo", Output: InlineValue("ok")},
	}

	require.NoError(t, store.Append(context.Background(), "run_1", events...))
	events[0] = Started{RunID: "run_other", WorkflowName: "mutated"}

	stored, ok, err := store.Events(context.Background(), "run_1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, Started{RunID: "run_1", WorkflowName: "echo"}, stored[0])

	stored[0] = Started{RunID: "run_other", WorkflowName: "returned slice mutated"}
	again, ok, err := store.Events(context.Background(), "run_1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, Started{RunID: "run_1", WorkflowName: "echo"}, again[0])
}

func TestMemoryRunStoreStateProjectsSuccessfulRun(t *testing.T) {
	store := NewMemoryRunStore()
	require.NoError(t, store.Append(context.Background(), "run_1",
		Started{RunID: "run_1", WorkflowName: "echo"},
		"retained but ignored non-workflow event",
		StepStarted{RunID: "run_1", WorkflowName: "echo", StepID: "echo", ActionName: "echo", Attempt: 1},
		StepCompleted{RunID: "run_1", WorkflowName: "echo", StepID: "echo", ActionName: "echo", Attempt: 1, Output: InlineValue("ok")},
		Completed{RunID: "run_1", WorkflowName: "echo", Output: InlineValue("ok")},
	))

	state, ok, err := store.State(context.Background(), "run_1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, RunSucceeded, state.Status)
	require.Equal(t, InlineValue("ok"), state.Output)
	require.Equal(t, StepSucceeded, state.Steps["echo"].Status)

	events, ok, err := store.Events(context.Background(), "run_1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Contains(t, events, "retained but ignored non-workflow event")
}

func TestMemoryRunStoreKeepsRunIDsSeparate(t *testing.T) {
	store := NewMemoryRunStore()
	require.NoError(t, store.Append(context.Background(), "run_1",
		Started{RunID: "run_1", WorkflowName: "echo"},
		Completed{RunID: "run_1", WorkflowName: "echo", Output: InlineValue("one")},
	))
	require.NoError(t, store.Append(context.Background(), "run_2",
		Started{RunID: "run_2", WorkflowName: "echo"},
		Completed{RunID: "run_2", WorkflowName: "echo", Output: InlineValue("two")},
	))

	state1, ok, err := store.State(context.Background(), "run_1")
	require.NoError(t, err)
	require.True(t, ok)
	state2, ok, err := store.State(context.Background(), "run_2")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, InlineValue("one"), state1.Output)
	require.Equal(t, InlineValue("two"), state2.Output)
}

func TestMemoryRunStoreUnknownRunReturnsNotFound(t *testing.T) {
	store := NewMemoryRunStore()

	events, ok, err := store.Events(context.Background(), "missing")
	require.NoError(t, err)
	require.False(t, ok)
	require.Nil(t, events)

	state, ok, err := store.State(context.Background(), "missing")
	require.NoError(t, err)
	require.False(t, ok)
	require.Equal(t, RunState{}, state)
}

func TestMemoryRunStoreStateSurfacesProjectorErrors(t *testing.T) {
	store := NewMemoryRunStore()
	require.NoError(t, store.Append(context.Background(), "run_1", &Started{RunID: "run_1", WorkflowName: "echo"}))

	_, ok, err := store.State(context.Background(), "run_1")
	require.False(t, ok)
	require.Error(t, err)
}
