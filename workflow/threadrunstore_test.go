package workflow

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/codewandler/agentsdk/thread"
	"github.com/stretchr/testify/require"
)

func TestWorkflowEventForThreadEventDecodesConcreteEvents(t *testing.T) {
	threadEvent, ok, err := ThreadEventForWorkflowEvent(StepCompleted{RunID: "run_1", WorkflowName: "echo", StepID: "echo", ActionName: "echo", Attempt: 1, Output: InlineValue("ok")})
	require.NoError(t, err)
	require.True(t, ok)

	payload, ok, err := WorkflowEventForThreadEvent(threadEvent)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, StepCompleted{RunID: "run_1", WorkflowName: "echo", StepID: "echo", ActionName: "echo", Attempt: 1, Output: InlineValue("ok")}, payload)
}

func TestWorkflowEventForThreadEventIgnoresNonWorkflowThreadEvents(t *testing.T) {
	payload, ok, err := WorkflowEventForThreadEvent(thread.Event{Kind: thread.EventThreadCreated})
	require.NoError(t, err)
	require.False(t, ok)
	require.Nil(t, payload)
}

func TestWorkflowEventForThreadEventRejectsInvalidPayload(t *testing.T) {
	_, ok, err := WorkflowEventForThreadEvent(thread.Event{Kind: EventStarted, Payload: json.RawMessage(`[]`)})
	require.True(t, ok)
	require.Error(t, err)
}

func TestThreadRunStoreAppendAndEvents(t *testing.T) {
	ctx := context.Background()
	store, live := newWorkflowThreadStore(t, ctx)
	runs := ThreadRunStore{Store: store, Live: live, ThreadID: live.ID()}

	require.NoError(t, runs.Append(ctx, "run_1",
		Started{RunID: "run_1", WorkflowName: "echo"},
		"ignored action event",
		Completed{RunID: "run_1", WorkflowName: "echo", Output: InlineValue("ok")},
	))

	events, ok, err := runs.Events(ctx, "run_1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, []any{
		Started{RunID: "run_1", WorkflowName: "echo"},
		Completed{RunID: "run_1", WorkflowName: "echo", Output: InlineValue("ok")},
	}, events)
}

func TestThreadRunStoreStateProjectsSuccessfulRun(t *testing.T) {
	ctx := context.Background()
	store, live := newWorkflowThreadStore(t, ctx)
	runs := ThreadRunStore{Store: store, Live: live, ThreadID: live.ID()}

	require.NoError(t, runs.Append(ctx, "run_1",
		Started{RunID: "run_1", WorkflowName: "echo"},
		StepStarted{RunID: "run_1", WorkflowName: "echo", StepID: "echo", ActionName: "echo", Attempt: 1},
		StepCompleted{RunID: "run_1", WorkflowName: "echo", StepID: "echo", ActionName: "echo", Attempt: 1, Output: InlineValue("ok")},
		Completed{RunID: "run_1", WorkflowName: "echo", Output: InlineValue("ok")},
	))

	state, ok, err := runs.State(ctx, "run_1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, RunSucceeded, state.Status)
	require.Equal(t, InlineValue("ok"), state.Output)
	require.Equal(t, StepSucceeded, state.Steps["echo"].Status)
}

func TestThreadRunStoreKeepsRunIDsSeparate(t *testing.T) {
	ctx := context.Background()
	store, live := newWorkflowThreadStore(t, ctx)
	runs := ThreadRunStore{Store: store, Live: live, ThreadID: live.ID()}

	require.NoError(t, runs.Append(ctx, "run_1",
		Started{RunID: "run_1", WorkflowName: "echo"},
		Completed{RunID: "run_1", WorkflowName: "echo", Output: InlineValue("one")},
	))
	require.NoError(t, runs.Append(ctx, "run_2",
		Started{RunID: "run_2", WorkflowName: "echo"},
		Completed{RunID: "run_2", WorkflowName: "echo", Output: InlineValue("two")},
	))

	events, ok, err := runs.Events(ctx, "run_1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, []any{
		Started{RunID: "run_1", WorkflowName: "echo"},
		Completed{RunID: "run_1", WorkflowName: "echo", Output: InlineValue("one")},
	}, events)
}

func TestThreadRunStoreRunsReturnsProjectedSummaries(t *testing.T) {
	ctx := context.Background()
	store, live := newWorkflowThreadStore(t, ctx)
	runs := ThreadRunStore{Store: store, Live: live, ThreadID: live.ID()}

	require.NoError(t, runs.Append(ctx, "run_b",
		Started{RunID: "run_b", WorkflowName: "echo"},
		Failed{RunID: "run_b", WorkflowName: "echo", Error: "boom"},
	))
	require.NoError(t, runs.Append(ctx, "run_a",
		Started{RunID: "run_a", WorkflowName: "ask"},
		Completed{RunID: "run_a", WorkflowName: "ask", Output: InlineValue("ok")},
	))

	summaries, err := runs.Runs(ctx)
	require.NoError(t, err)
	require.Equal(t, []RunSummary{
		{ID: "run_a", WorkflowName: "ask", Status: RunSucceeded},
		{ID: "run_b", WorkflowName: "echo", Status: RunFailed, Error: "boom"},
	}, summaries)
}

func TestThreadRunStoreRunsReturnsEmptyWhenNoWorkflowEvents(t *testing.T) {
	ctx := context.Background()
	store, live := newWorkflowThreadStore(t, ctx)
	runs := ThreadRunStore{Store: store, Live: live, ThreadID: live.ID()}

	summaries, err := runs.Runs(ctx)
	require.NoError(t, err)
	require.Empty(t, summaries)
}

func TestThreadRunStoreUnknownRunReturnsNotFound(t *testing.T) {
	ctx := context.Background()
	store, live := newWorkflowThreadStore(t, ctx)
	runs := ThreadRunStore{Store: store, Live: live, ThreadID: live.ID()}

	events, ok, err := runs.Events(ctx, "missing")
	require.NoError(t, err)
	require.False(t, ok)
	require.Nil(t, events)

	state, ok, err := runs.State(ctx, "missing")
	require.NoError(t, err)
	require.False(t, ok)
	require.Equal(t, RunState{}, state)
}

func TestThreadRunStoreRejectsMismatchedRunIDOnAppend(t *testing.T) {
	ctx := context.Background()
	store, live := newWorkflowThreadStore(t, ctx)
	runs := ThreadRunStore{Store: store, Live: live, ThreadID: live.ID()}

	err := runs.Append(ctx, "run_1", Started{RunID: "run_2", WorkflowName: "echo"})
	require.Error(t, err)
}

func TestThreadRunStoreFiltersBranches(t *testing.T) {
	ctx := context.Background()
	store, mainLive := newWorkflowThreadStore(t, ctx)
	mainRuns := ThreadRunStore{Store: store, Live: mainLive, ThreadID: mainLive.ID(), BranchID: thread.MainBranch}
	require.NoError(t, mainRuns.Append(ctx, "run_before_fork", Started{RunID: "run_before_fork", WorkflowName: "echo"}))

	altLive, err := store.Fork(ctx, thread.ForkParams{ID: mainLive.ID(), FromBranchID: thread.MainBranch, ToBranchID: "alt"})
	require.NoError(t, err)
	altRuns := ThreadRunStore{Store: store, Live: altLive, ThreadID: mainLive.ID(), BranchID: "alt"}
	require.NoError(t, altRuns.Append(ctx, "run_alt", Started{RunID: "run_alt", WorkflowName: "echo"}))
	require.NoError(t, mainRuns.Append(ctx, "run_after_fork", Started{RunID: "run_after_fork", WorkflowName: "echo"}))

	_, ok, err := mainRuns.Events(ctx, "run_alt")
	require.NoError(t, err)
	require.False(t, ok)

	_, ok, err = altRuns.Events(ctx, "run_after_fork")
	require.NoError(t, err)
	require.False(t, ok)

	events, ok, err := altRuns.Events(ctx, "run_before_fork")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, []any{Started{RunID: "run_before_fork", WorkflowName: "echo"}}, events)
}

func newWorkflowThreadStore(t *testing.T, ctx context.Context) (*thread.MemoryStore, thread.Live) {
	t.Helper()
	registry, err := thread.NewEventRegistry(append(thread.CoreEventDefinitions(), EventDefinitions()...)...)
	require.NoError(t, err)
	store := thread.NewMemoryStore(thread.WithEventRegistry(registry))
	live, err := store.Create(ctx, thread.CreateParams{ID: "thread_1"})
	require.NoError(t, err)
	return store, live
}
