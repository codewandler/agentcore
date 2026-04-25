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
	commitAssistantTurn(t, s, "hello", "hi", NewProviderContinuation(
		ProviderIdentity{ProviderName: "openai", APIKind: "openai.responses", APIFamily: "openai.responses", NativeModel: "gpt-test"},
		"resp_1",
		unified.Extensions{},
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
	commitAssistantTurn(t, s, "hello", "hi", NewProviderContinuation(
		ProviderIdentity{ProviderName: "openai", APIKind: "openai.responses", NativeModel: "gpt-test"},
		"resp_1",
		unified.Extensions{},
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

func TestSessionNativeContinuationMatchesResponsesAliases(t *testing.T) {
	s := New()
	commitAssistantTurn(t, s, "hello", "hi", NewProviderContinuation(
		ProviderIdentity{ProviderName: "openai", APIKind: "openai.responses", NativeModel: "gpt-test"},
		"resp_1",
		unified.Extensions{},
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
	commitAssistantTurn(t, s, "root", "root reply", NewProviderContinuation(
		ProviderIdentity{ProviderName: "openai", APIKind: "openai.responses", NativeModel: "gpt-test"},
		"resp_root",
		unified.Extensions{},
	))
	require.NoError(t, s.Fork("alt"))

	require.NoError(t, s.Checkout(MainBranch))
	commitAssistantTurn(t, s, "main", "main reply", NewProviderContinuation(
		ProviderIdentity{ProviderName: "openai", APIKind: "openai.responses", NativeModel: "gpt-test"},
		"resp_main",
		unified.Extensions{},
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

func requireText(t *testing.T, msg unified.Message, want string) {
	t.Helper()
	require.Len(t, msg.Content, 1)
	text, ok := msg.Content[0].(unified.TextPart)
	require.True(t, ok)
	require.Equal(t, want, text.Text)
}
