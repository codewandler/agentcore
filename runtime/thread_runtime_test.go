package runtime

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/codewandler/agentsdk/capabilities/planner"
	"github.com/codewandler/agentsdk/capability"
	"github.com/codewandler/agentsdk/conversation"
	"github.com/codewandler/agentsdk/thread"
	"github.com/codewandler/llmadapter/unified"
	"github.com/stretchr/testify/require"
)

func TestThreadRuntimeInjectsPlannerToolsContextAndResumes(t *testing.T) {
	ctx := context.Background()
	store := thread.NewMemoryStore()
	live, err := store.Create(ctx, thread.CreateParams{
		ID:     "thread_runtime",
		Source: thread.EventSource{Type: "session", SessionID: "session_1"},
	})
	require.NoError(t, err)
	registry, err := capability.NewRegistry(planner.Factory{})
	require.NoError(t, err)
	threadRuntime, err := NewThreadRuntime(live, registry, WithThreadRuntimeSource(thread.EventSource{Type: "session", SessionID: "session_1"}))
	require.NoError(t, err)
	_, err = threadRuntime.AttachCapability(ctx, capability.AttachSpec{CapabilityName: planner.CapabilityName, InstanceID: "planner_1"})
	require.NoError(t, err)

	client := &fakeClient{events: [][]unified.Event{
		{
			unified.ToolCallDoneEvent{Index: 0, ID: "call_plan", Name: "plan", Args: json.RawMessage(`{"actions":[
				{"action":"create_plan","plan":{"id":"plan_1","title":"Runtime plan"}},
				{"action":"add_step","step":{"id":"step_1","title":"Persist planner state","status":"in_progress"}}
			]}`)},
			unified.CompletedEvent{FinishReason: unified.FinishReasonToolCall},
		},
		{
			unified.TextDeltaEvent{Text: "done"},
			unified.CompletedEvent{FinishReason: unified.FinishReasonStop},
		},
	}}
	engine, err := New(client,
		WithThreadRuntime(threadRuntime),
		WithMaxSteps(2),
		WithToolChoice(unified.ToolChoice{Mode: unified.ToolChoiceAuto}),
	)
	require.NoError(t, err)

	_, err = engine.RunTurn(ctx, "create a plan")
	require.NoError(t, err)
	require.Len(t, client.requests, 2)
	requireToolSpec(t, client.requests[0], "plan")
	requireNoMessageContaining(t, client.requests[0], "Runtime plan")
	requireMessageContaining(t, client.requests[1], "Plan \"Runtime plan\" has 1 step(s).")
	requireMessageContaining(t, client.requests[1], "title: Persist planner state")
	sessionMessages, err := engine.Session().Messages()
	require.NoError(t, err)
	requireNoStoredMessageContaining(t, sessionMessages, "Runtime plan")

	stored, err := store.Read(ctx, thread.ReadParams{ID: live.ID()})
	require.NoError(t, err)
	requireEventCountRuntime(t, stored.Events, capability.EventAttached, 1)
	requireEventCountRuntime(t, stored.Events, capability.EventStateEventDispatched, 2)

	resumedRuntime, _, err := ResumeThreadRuntime(ctx, store, thread.ResumeParams{
		ID:     live.ID(),
		Source: thread.EventSource{Type: "session", SessionID: "session_2"},
	}, registry)
	require.NoError(t, err)
	resumedClient := &fakeClient{events: [][]unified.Event{{
		unified.TextDeltaEvent{Text: "resumed"},
		unified.CompletedEvent{FinishReason: unified.FinishReasonStop},
	}}}
	resumedEngine, err := New(resumedClient,
		WithSession(conversation.New(conversation.WithSessionID("session_2"))),
		WithThreadRuntime(resumedRuntime),
	)
	require.NoError(t, err)

	_, err = resumedEngine.RunTurn(ctx, "continue")
	require.NoError(t, err)
	require.Len(t, resumedClient.requests, 1)
	requireToolSpec(t, resumedClient.requests[0], "plan")
	requireMessageContaining(t, resumedClient.requests[0], "Plan \"Runtime plan\" has 1 step(s).")
	requireMessageContaining(t, resumedClient.requests[0], "title: Persist planner state")
}

func requireToolSpec(t *testing.T, req unified.Request, name string) {
	t.Helper()
	for _, spec := range req.Tools {
		if spec.Name == name {
			return
		}
	}
	t.Fatalf("missing tool spec %q in %#v", name, req.Tools)
}

func requireMessageContaining(t *testing.T, req unified.Request, want string) {
	t.Helper()
	for _, message := range req.Messages {
		for _, part := range message.Content {
			text, ok := part.(unified.TextPart)
			if ok && strings.Contains(text.Text, want) {
				return
			}
		}
	}
	t.Fatalf("missing message containing %q in %#v", want, req.Messages)
}

func requireNoMessageContaining(t *testing.T, req unified.Request, want string) {
	t.Helper()
	for _, message := range req.Messages {
		for _, part := range message.Content {
			text, ok := part.(unified.TextPart)
			if ok && strings.Contains(text.Text, want) {
				t.Fatalf("unexpected message containing %q in %#v", want, req.Messages)
			}
		}
	}
}

func requireNoStoredMessageContaining(t *testing.T, messages []unified.Message, want string) {
	t.Helper()
	for _, message := range messages {
		for _, part := range message.Content {
			text, ok := part.(unified.TextPart)
			if ok && strings.Contains(text.Text, want) {
				t.Fatalf("unexpected stored message containing %q in %#v", want, messages)
			}
		}
	}
}

func requireEventCountRuntime(t *testing.T, events []thread.Event, kind thread.EventKind, want int) {
	t.Helper()
	var got int
	for _, event := range events {
		if event.Kind == kind {
			got++
		}
	}
	require.Equal(t, want, got, "event count for %q", kind)
}
