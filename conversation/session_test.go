package conversation

import (
	"testing"

	"github.com/codewandler/llmadapter/unified"
	"github.com/stretchr/testify/require"
)

func TestSessionReplayProjection(t *testing.T) {
	s := New(WithModel("model-a"), WithSystem("system"))
	_, err := s.AddUser("hello")
	require.NoError(t, err)
	_, err = s.AppendMessage(unified.Message{Role: unified.RoleAssistant, ID: "resp_1", Content: []unified.ContentPart{unified.TextPart{Text: "hi"}}})
	require.NoError(t, err)

	req, err := s.BuildRequest(NewRequest().User("next").Stream(true).Build())
	require.NoError(t, err)

	require.Equal(t, "model-a", req.Model)
	require.True(t, req.Stream)
	require.Equal(t, unified.CachePolicyOn, req.CachePolicy)
	require.Len(t, req.Instructions, 1)
	require.Len(t, req.Messages, 3)
	require.Equal(t, unified.RoleUser, req.Messages[0].Role)
	require.Equal(t, unified.RoleAssistant, req.Messages[1].Role)
	require.Empty(t, req.Messages[1].ID)
	require.Equal(t, unified.RoleUser, req.Messages[2].Role)
}

func TestSessionReplayProjectionStripsUnsignedReasoning(t *testing.T) {
	s := New()
	_, err := s.AppendMessage(unified.Message{
		Role: unified.RoleAssistant,
		Content: []unified.ContentPart{
			unified.ReasoningPart{Text: "unsigned"},
			unified.TextPart{Text: "visible"},
		},
	})
	require.NoError(t, err)

	req, err := s.BuildRequest(NewRequest().User("next").Build())
	require.NoError(t, err)

	require.Len(t, req.Messages, 2)
	require.Len(t, req.Messages[0].Content, 1)
	requireText(t, req.Messages[0], "visible")
}

func TestSessionReplayProjectionKeepsSignedReasoning(t *testing.T) {
	s := New()
	_, err := s.AppendMessage(unified.Message{
		Role: unified.RoleAssistant,
		Content: []unified.ContentPart{
			unified.ReasoningPart{Text: "signed", Signature: "sig"},
			unified.TextPart{Text: "visible"},
		},
	})
	require.NoError(t, err)

	req, err := s.BuildRequest(NewRequest().User("next").Build())
	require.NoError(t, err)

	require.Len(t, req.Messages, 2)
	require.Len(t, req.Messages[0].Content, 2)
	reasoning, ok := req.Messages[0].Content[0].(unified.ReasoningPart)
	require.True(t, ok)
	require.Equal(t, "signed", reasoning.Text)
	require.Equal(t, "sig", reasoning.Signature)
}

func TestSessionCachePolicyCanBeOverridden(t *testing.T) {
	s := New(WithCacheKey("session-key"), WithCacheTTL("1h"))

	req, err := s.BuildRequest(NewRequest().CachePolicy(unified.CachePolicyOff).Build())
	require.NoError(t, err)

	require.Equal(t, unified.CachePolicyOff, req.CachePolicy)
	require.Equal(t, "session-key", req.CacheKey)
	require.Equal(t, "1h", req.CacheTTL)
}

func TestSessionNativeContinuationProjection(t *testing.T) {
	s := New(WithModel("model-a"))
	commitAssistantTurn(t, s, "hello", "hi", nativeContinuation(
		ProviderIdentity{ProviderName: "openai", APIKind: "openai.responses", APIFamily: "openai.responses", NativeModel: "gpt-test"},
		"resp_1",
	))

	req, err := s.BuildRequestForProvider(NewRequest().User("next").Build(), ProviderIdentity{
		ProviderName: "openai",
		APIKind:      "openai.responses",
		APIFamily:    "openai.responses",
		NativeModel:  "gpt-test",
	})
	require.NoError(t, err)

	require.Equal(t, "model-a", req.Model)
	require.Len(t, req.Messages, 1)
	require.Equal(t, unified.RoleUser, req.Messages[0].Role)
	requireText(t, req.Messages[0], "next")
	previousResponseID, ok, err := unified.GetExtension[string](req.Extensions, unified.ExtOpenAIPreviousResponseID)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "resp_1", previousResponseID)
}

func TestSessionNativeContinuationFallsBackToReplayOnProviderMismatch(t *testing.T) {
	s := New()
	commitAssistantTurn(t, s, "hello", "hi", nativeContinuation(
		ProviderIdentity{ProviderName: "openai", APIKind: "openai.responses", NativeModel: "gpt-test"},
		"resp_1",
	))

	req, err := s.BuildRequestForProvider(NewRequest().User("next").Build(), ProviderIdentity{
		ProviderName: "openai",
		APIKind:      "openai.responses",
		NativeModel:  "gpt-other",
	})
	require.NoError(t, err)

	require.Len(t, req.Messages, 3)
	require.False(t, req.Extensions.Has(unified.ExtOpenAIPreviousResponseID))
}

func TestSessionNativeContinuationFallsBackForUnsupportedResponsesFamily(t *testing.T) {
	s := New()
	commitAssistantTurn(t, s, "hello", "hi", NewProviderContinuation(
		ProviderIdentity{ProviderName: "codex_responses", APIKind: "codex.responses", NativeModel: "gpt-test"},
		"resp_1",
		unified.Extensions{},
	))

	req, err := s.BuildRequestForProvider(NewRequest().User("next").Build(), ProviderIdentity{
		ProviderName: "codex_responses",
		APIKind:      "codex.responses",
		NativeModel:  "gpt-test",
	})
	require.NoError(t, err)

	require.Len(t, req.Messages, 3)
	require.False(t, req.Extensions.Has(unified.ExtOpenAIPreviousResponseID))
}

func TestSessionContinuationRequiresRouteMetadata(t *testing.T) {
	s := New()
	commitAssistantTurn(t, s, "hello", "hi", NewProviderContinuation(
		ProviderIdentity{ProviderName: "openai", APIKind: "openai.responses", NativeModel: "gpt-test"},
		"resp_1",
		unified.Extensions{},
	))

	req, err := s.BuildRequestForProvider(NewRequest().User("next").Build(), ProviderIdentity{
		ProviderName: "openai",
		APIKind:      "openai.responses",
		NativeModel:  "gpt-test",
	})
	require.NoError(t, err)
	require.Len(t, req.Messages, 3)
	require.False(t, req.Extensions.Has(unified.ExtOpenAIPreviousResponseID))
}

func TestSessionCodexProjectionAddsSessionHintsAndKeepsReplay(t *testing.T) {
	s := New(WithConversationID("thread_1"), WithSessionID("live_1"))
	commitAssistantTurn(t, s, "hello", "hi", NewProviderContinuation(
		ProviderIdentity{ProviderName: "codex_responses", APIKind: "codex.responses", NativeModel: "gpt-test"},
		"resp_1",
		unified.Extensions{},
	))

	req, err := s.BuildRequestForProvider(NewRequest().User("next").Build(), ProviderIdentity{
		ProviderName: "codex_responses",
		APIKind:      "codex.responses",
		NativeModel:  "gpt-test",
	})
	require.NoError(t, err)
	require.Len(t, req.Messages, 3)
	require.False(t, req.Extensions.Has(unified.ExtOpenAIPreviousResponseID))

	codex, warnings := unified.CodexExtensionsFrom(req.Extensions)
	require.Empty(t, warnings)
	require.Equal(t, unified.InteractionSession, codex.InteractionMode)
	require.Equal(t, "thread_1", codex.SessionID)
	require.Equal(t, string(MainBranch), codex.BranchID)
	require.NotEmpty(t, codex.BranchHeadID)
	require.Equal(t, "resp_1", codex.ParentResponseID)
}

func TestSessionNativeContinuationMatchesResponsesAliases(t *testing.T) {
	s := New()
	commitAssistantTurn(t, s, "hello", "hi", nativeContinuation(
		ProviderIdentity{ProviderName: "openai", APIKind: "openai.responses", NativeModel: "gpt-test"},
		"resp_1",
	))

	req, err := s.BuildRequestForProvider(NewRequest().User("next").Build(), ProviderIdentity{
		ProviderName: "openai",
		APIKind:      "responses",
		NativeModel:  "gpt-test",
	})
	require.NoError(t, err)

	require.Len(t, req.Messages, 1)
	previousResponseID, ok, err := unified.GetExtension[string](req.Extensions, unified.ExtOpenAIPreviousResponseID)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "resp_1", previousResponseID)
}

func TestSessionNativeContinuationUsesSelectedBranchHead(t *testing.T) {
	s := New()
	commitAssistantTurn(t, s, "root", "root reply", nativeContinuation(
		ProviderIdentity{ProviderName: "openai", APIKind: "openai.responses", NativeModel: "gpt-test"},
		"resp_root",
	))
	require.NoError(t, s.Fork("alt"))

	require.NoError(t, s.Checkout(MainBranch))
	commitAssistantTurn(t, s, "main", "main reply", nativeContinuation(
		ProviderIdentity{ProviderName: "openai", APIKind: "openai.responses", NativeModel: "gpt-test"},
		"resp_main",
	))

	require.NoError(t, s.Checkout("alt"))
	req, err := s.BuildRequestForProvider(NewRequest().User("alt next").Build(), ProviderIdentity{
		ProviderName: "openai",
		APIKind:      "openai.responses",
		NativeModel:  "gpt-test",
	})
	require.NoError(t, err)

	require.Len(t, req.Messages, 1)
	requireText(t, req.Messages[0], "alt next")
	previousResponseID, ok, err := unified.GetExtension[string](req.Extensions, unified.ExtOpenAIPreviousResponseID)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "resp_root", previousResponseID)
}

func TestSessionProjectionPolicyOverride(t *testing.T) {
	var seen ProjectionInput
	s := New(WithProjectionPolicy(ProjectionPolicyFunc(func(input ProjectionInput) (ProjectionResult, error) {
		seen = input
		return ProjectionResult{
			Messages: []unified.Message{{
				Role:    unified.RoleUser,
				Content: []unified.ContentPart{unified.TextPart{Text: "projected"}},
			}},
			Extensions: cloneExtensions(input.Extensions),
		}, nil
	})))

	req, err := s.BuildRequest(NewRequest().User("ignored").Build())
	require.NoError(t, err)

	require.Len(t, req.Messages, 1)
	requireText(t, req.Messages[0], "projected")
	require.Empty(t, seen.Items)
	require.Len(t, seen.PendingItems, 1)
	requireText(t, seen.PendingItems[0].Message, "ignored")
}

func TestSessionProjectsPendingContextItems(t *testing.T) {
	s := New()
	req, err := s.BuildRequest(Request{
		Items: []Item{{
			Kind: ItemContextFragment,
			Message: unified.Message{
				Role:    unified.RoleUser,
				Name:    "context",
				Content: []unified.ContentPart{unified.TextPart{Text: "context item"}},
			},
		}},
		Messages: []unified.Message{{
			Role:    unified.RoleUser,
			Content: []unified.ContentPart{unified.TextPart{Text: "user text"}},
		}},
	})
	require.NoError(t, err)
	require.Len(t, req.Messages, 2)
	requireText(t, req.Messages[0], "context item")
	requireText(t, req.Messages[1], "user text")
}

func TestSessionCompactionSuppressesReplacedNodes(t *testing.T) {
	s := New()
	oldOne, err := s.AddUser("old one")
	require.NoError(t, err)
	oldTwo, err := s.AddUser("old two")
	require.NoError(t, err)
	keep, err := s.AddUser("keep")
	require.NoError(t, err)

	compaction, err := s.Compact("summary of old messages", oldOne, oldTwo)
	require.NoError(t, err)

	items, err := ProjectItems(s.Tree(), s.Branch())
	require.NoError(t, err)
	require.Len(t, items, 2)
	require.Equal(t, ItemMessage, items[0].Kind)
	require.Equal(t, keep, items[0].NodeID)
	require.Equal(t, ItemCompaction, items[1].Kind)
	require.NotNil(t, items[1].Compaction)
	require.Equal(t, []NodeID{oldOne, oldTwo}, items[1].Compaction.Replaces)

	req, err := s.BuildRequest(NewRequest().User("next").Build())
	require.NoError(t, err)
	require.Len(t, req.Messages, 3)
	requireText(t, req.Messages[0], "keep")
	require.Equal(t, unified.RoleUser, req.Messages[1].Role)
	require.Equal(t, "conversation_summary", req.Messages[1].Name)
	requireText(t, req.Messages[1], "summary of old messages")
	requireText(t, req.Messages[2], "next")

	_, ok := s.Tree().Node(oldOne)
	require.True(t, ok)
	_, ok = s.Tree().Node(oldTwo)
	require.True(t, ok)
	_, ok = s.Tree().Node(compaction)
	require.True(t, ok)
}

func TestSessionCompactionCanReplaceEarlierCompaction(t *testing.T) {
	s := New()
	oldOne, err := s.AddUser("old one")
	require.NoError(t, err)
	firstSummary, err := s.Compact("first summary", oldOne)
	require.NoError(t, err)

	_, err = s.Compact("second summary", firstSummary)
	require.NoError(t, err)

	messages, err := s.Messages()
	require.NoError(t, err)
	require.Len(t, messages, 1)
	requireText(t, messages[0], "second summary")
}

func TestSessionCompactionValidation(t *testing.T) {
	s := New()
	_, err := s.Compact("   ")
	require.Error(t, err)

	_, err = s.Compact("summary", "missing")
	require.Error(t, err)

	id, err := s.AddUser("kept")
	require.NoError(t, err)
	_, err = s.Compact("summary", id)
	require.NoError(t, err)
}

func TestSessionCompactionPersistsThroughResume(t *testing.T) {
	store := NewMemoryStore()
	original := New(WithStore(store))
	oldOne, err := original.AddUser("old one")
	require.NoError(t, err)
	oldTwo, err := original.AddUser("old two")
	require.NoError(t, err)
	_, err = original.Compact("summary of old messages", oldOne, oldTwo)
	require.NoError(t, err)

	resumed, err := Resume(t.Context(), store, original.ConversationID())
	require.NoError(t, err)
	messages, err := resumed.Messages()
	require.NoError(t, err)
	require.Len(t, messages, 1)
	requireText(t, messages[0], "summary of old messages")
}

func TestSessionForkUsesSelectedBranchPath(t *testing.T) {
	s := New()
	_, err := s.AddUser("root")
	require.NoError(t, err)

	require.NoError(t, s.Fork("alt"))
	_, err = s.AddUser("alt only")
	require.NoError(t, err)

	require.NoError(t, s.Checkout(MainBranch))
	_, err = s.AddUser("main only")
	require.NoError(t, err)

	mainMsgs, err := s.Messages()
	require.NoError(t, err)
	require.Len(t, mainMsgs, 2)
	requireText(t, mainMsgs[1], "main only")

	require.NoError(t, s.Checkout("alt"))
	altMsgs, err := s.Messages()
	require.NoError(t, err)
	require.Len(t, altMsgs, 2)
	requireText(t, altMsgs[1], "alt only")
}

func commitAssistantTurn(t *testing.T, s *Session, userText string, assistantText string, continuation ProviderContinuation) {
	t.Helper()
	fragment := NewTurnFragment()
	fragment.AddRequestMessages(unified.Message{
		Role:    unified.RoleUser,
		Content: []unified.ContentPart{unified.TextPart{Text: userText}},
	})
	fragment.SetAssistantMessage(unified.Message{
		Role:    unified.RoleAssistant,
		Content: []unified.ContentPart{unified.TextPart{Text: assistantText}},
	})
	fragment.AddContinuation(continuation)
	fragment.Complete(unified.FinishReasonStop)
	_, err := s.CommitFragment(fragment)
	require.NoError(t, err)
}

func nativeContinuation(identity ProviderIdentity, responseID string) ProviderContinuation {
	continuation := NewProviderContinuation(identity, responseID, unified.Extensions{})
	continuation.ConsumerContinuation = unified.ContinuationPreviousResponseID
	continuation.InternalContinuation = unified.ContinuationPreviousResponseID
	continuation.Transport = unified.TransportHTTPSSE
	return continuation
}

func requireText(t *testing.T, msg unified.Message, want string) {
	t.Helper()
	require.Len(t, msg.Content, 1)
	text, ok := msg.Content[0].(unified.TextPart)
	require.True(t, ok)
	require.Equal(t, want, text.Text)
}
