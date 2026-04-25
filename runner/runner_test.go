package runner

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/codewandler/agentsdk/conversation"
	"github.com/codewandler/agentsdk/runnertest"
	"github.com/codewandler/agentsdk/tool"
	"github.com/codewandler/llmadapter/unified"
	"github.com/stretchr/testify/require"
)

func TestRunTurnCommitsOnlyAfterFinalResponse(t *testing.T) {
	client := runnertest.NewClient(
		[]unified.Event{
			runnertest.Route("openai", "openai.responses", "openai.responses", "public", "gpt-test"),
			unified.TextDeltaEvent{Text: "hello"},
			unified.CompletedEvent{FinishReason: unified.FinishReasonStop, MessageID: "resp_1"},
		},
	)
	sess := conversation.New()

	result, err := RunTurn(context.Background(), sess, client, conversation.NewRequest().User("hi").Build(),
		WithProviderIdentity(conversation.ProviderIdentity{ProviderName: "test", APIKind: "responses"}),
	)
	require.NoError(t, err)
	requireEventType[StepStartEvent](t, result.Events)
	requireEventType[StepDoneEvent](t, result.Events)
	requireEventType[TextDeltaEvent](t, result.Events)
	requireEventType[CompletedEvent](t, result.Events)
	route := requireEventType[RouteEvent](t, result.Events)
	require.Equal(t, "openai", route.ProviderIdentity.ProviderName)

	messages, err := sess.Messages()
	require.NoError(t, err)
	require.Len(t, messages, 2)
	require.Equal(t, unified.RoleUser, messages[0].Role)
	require.Equal(t, unified.RoleAssistant, messages[1].Role)
	require.Empty(t, messages[1].ID)

	continuation, ok, err := conversation.ContinuationAtHead(sess.Tree(), sess.Branch(), conversation.ProviderIdentity{ProviderName: "openai"})
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "resp_1", continuation.ResponseID)
	require.Equal(t, "gpt-test", continuation.NativeModel)
}

func TestRunTurnUsesNativeContinuationProjection(t *testing.T) {
	client := runnertest.NewClient(runnertest.TextStream("next", "resp_2"))
	sess := conversation.New()
	fragment := conversation.NewTurnFragment()
	fragment.AddRequestMessages(unified.Message{
		Role:    unified.RoleUser,
		Content: []unified.ContentPart{unified.TextPart{Text: "hello"}},
	})
	fragment.SetAssistantMessage(unified.Message{
		Role:    unified.RoleAssistant,
		Content: []unified.ContentPart{unified.TextPart{Text: "hi"}},
	})
	fragment.AddContinuation(conversation.NewProviderContinuation(
		conversation.ProviderIdentity{ProviderName: "openai", APIKind: "openai.responses", NativeModel: "gpt-test"},
		"resp_1",
		unified.Extensions{},
	))
	fragment.Complete(unified.FinishReasonStop)
	_, err := sess.CommitFragment(fragment)
	require.NoError(t, err)

	_, err = RunTurn(context.Background(), sess, client, conversation.NewRequest().User("again").Build(),
		WithProviderIdentity(conversation.ProviderIdentity{ProviderName: "openai", APIKind: "openai.responses", NativeModel: "gpt-test"}),
	)
	require.NoError(t, err)
	require.Len(t, client.Requests(), 1)
	require.Len(t, client.RequestAt(0).Messages, 1)
	previousResponseID, ok, err := unified.GetExtension[string](client.RequestAt(0).Extensions, unified.ExtOpenAIPreviousResponseID)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "resp_1", previousResponseID)
}

func TestRunTurnPreservesReasoningSignatureForReplay(t *testing.T) {
	client := runnertest.NewClient(runnertest.ReasoningTextStream("think", "sig", "hello"))
	sess := conversation.New()

	_, err := RunTurn(context.Background(), sess, client, conversation.NewRequest().User("hi").Build())
	require.NoError(t, err)

	messages, err := sess.Messages()
	require.NoError(t, err)
	require.Len(t, messages, 2)
	require.Len(t, messages[1].Content, 2)
	reasoning, ok := messages[1].Content[0].(unified.ReasoningPart)
	require.True(t, ok)
	require.Equal(t, "think", reasoning.Text)
	require.Equal(t, "sig", reasoning.Signature)
}

func TestRunTurnExecutesToolAndCommitsWholeTranscript(t *testing.T) {
	client := runnertest.NewClient(
		runnertest.ToolCallStream("resp_tool", runnertest.ToolCall("echo", "call_1", 0, `{"text":"ok"}`)),
		runnertest.TextStream("done", "resp_final"),
	)
	echo := tool.New("echo", "echo text", func(_ tool.Ctx, p struct {
		Text string `json:"text"`
	}) (tool.Result, error) {
		return tool.Text(p.Text), nil
	})
	sess := conversation.New(conversation.WithTools(tool.UnifiedToolsFrom([]tool.Tool{echo})))

	_, err := RunTurn(context.Background(), sess, client, conversation.NewRequest().User("use echo").Build(), WithTools([]tool.Tool{echo}))
	require.NoError(t, err)
	require.Len(t, client.Requests(), 2)
	require.Len(t, client.RequestAt(1).Messages, 3)

	messages, err := sess.Messages()
	require.NoError(t, err)
	require.Len(t, messages, 4)
	require.Len(t, messages[1].ToolCalls, 1)
	require.Len(t, messages[2].ToolResults, 1)
}

func TestRunTurnAccumulatesToolArgsDeltas(t *testing.T) {
	client := runnertest.NewClient(
		[]unified.Event{
			unified.ToolCallStartEvent{Index: 0, ID: "call_1", Name: "echo"},
			unified.ToolCallArgsDeltaEvent{Index: 0, Delta: `{"text"`},
			unified.ToolCallArgsDeltaEvent{Index: 0, Delta: `:"ok"}`},
			unified.ToolCallDoneEvent{Index: 0},
			unified.CompletedEvent{FinishReason: unified.FinishReasonToolCall},
		},
	)
	echo := tool.New("echo", "echo text", func(_ tool.Ctx, p struct {
		Text string `json:"text"`
	}) (tool.Result, error) {
		return tool.Text(p.Text), nil
	})

	result, err := RunTurn(context.Background(), conversation.New(), client, conversation.NewRequest().User("use echo").Build(),
		WithTools([]tool.Tool{echo}),
		WithMaxSteps(1),
	)
	require.ErrorIs(t, err, ErrMaxStepsReached)
	var toolResult ToolResultEvent
	for _, event := range result.Events {
		if ev, ok := event.(ToolResultEvent); ok {
			toolResult = ev
		}
	}
	require.Equal(t, "ok", toolResult.Output)
	require.False(t, toolResult.IsError)
}

func TestRunTurnToolTimeoutEmitsTimedOutResult(t *testing.T) {
	client := runnertest.NewClient(runnertest.ToolCallStream("resp_tool", runnertest.ToolCall("slow", "call_1", 0, `{}`)))
	slow := tool.New("slow", "slow tool", func(ctx tool.Ctx, _ struct{}) (tool.Result, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})

	result, err := RunTurn(context.Background(), conversation.New(), client, conversation.NewRequest().User("use slow").Build(),
		WithTools([]tool.Tool{slow}),
		WithToolTimeout(time.Millisecond),
		WithMaxSteps(1),
	)
	require.ErrorIs(t, err, ErrMaxStepsReached)
	requireToolResult(t, result.Events, "[Timed out]", true)
}

func TestRunTurnCancellationEmitsCanceledForRemainingToolCalls(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	client := runnertest.NewClient(runnertest.ToolCallStream("resp_tool",
		runnertest.ToolCall("first", "call_1", 0, `{}`),
		runnertest.ToolCall("second", "call_2", 1, `{}`),
	))
	executor := ToolExecutorFunc(func(_ context.Context, call unified.ToolCall) unified.ToolResult {
		if call.Name == "first" {
			cancel()
			return toolResult(call, "[Canceled]", true)
		}
		return toolResult(call, "should not run", false)
	})

	result, err := RunTurn(ctx, conversation.New(), client, conversation.NewRequest().User("use tools").Build(), WithToolExecutor(executor))
	require.ErrorIs(t, err, context.Canceled)
	var outputs []string
	for _, event := range result.Events {
		if ev, ok := event.(ToolResultEvent); ok {
			outputs = append(outputs, ev.Output)
		}
	}
	require.Equal(t, []string{"[Canceled]", "[Canceled]"}, outputs)
}

func TestRunTurnPassesThroughWarningsAndRawEvents(t *testing.T) {
	client := runnertest.NewClient(
		[]unified.Event{
			unified.WarningEvent{Code: "dropped", Message: "field dropped"},
			unified.RawEvent{APIKind: "test", Type: "raw"},
			unified.TextDeltaEvent{Text: "ok"},
			unified.CompletedEvent{FinishReason: unified.FinishReasonStop},
		},
	)
	result, err := RunTurn(context.Background(), conversation.New(), client, conversation.NewRequest().User("hi").Build())
	require.NoError(t, err)
	requireEventType[WarningEvent](t, result.Events)
	requireEventType[RawEvent](t, result.Events)
}

func TestRunTurnProviderErrorDoesNotCommit(t *testing.T) {
	client := runnertest.NewClient(runnertest.ErrorStream(errors.New("boom")))
	sess := conversation.New()
	_, err := RunTurn(context.Background(), sess, client, conversation.NewRequest().User("hi").Build())
	require.ErrorContains(t, err, "boom")
	messages, msgErr := sess.Messages()
	require.NoError(t, msgErr)
	require.Empty(t, messages)
}

func TestRunTurnIncompleteStreamDoesNotCommit(t *testing.T) {
	client := runnertest.NewClient(runnertest.IncompleteTextStream("partial"))
	sess := conversation.New()
	_, err := RunTurn(context.Background(), sess, client, conversation.NewRequest().User("hi").Build())
	require.ErrorContains(t, err, "without completed")
	messages, msgErr := sess.Messages()
	require.NoError(t, msgErr)
	require.Empty(t, messages)
}

func requireToolResult(t *testing.T, events []Event, output string, isError bool) {
	t.Helper()
	for _, event := range events {
		if ev, ok := event.(ToolResultEvent); ok {
			require.Equal(t, output, ev.Output)
			require.Equal(t, isError, ev.IsError)
			return
		}
	}
	require.Fail(t, "missing tool result event")
}

func requireEventType[T Event](t *testing.T, events []Event) T {
	t.Helper()
	for _, event := range events {
		if ev, ok := event.(T); ok {
			return ev
		}
	}
	var zero T
	require.Failf(t, "missing event type", "%T", zero)
	return zero
}
