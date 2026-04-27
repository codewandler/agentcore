package conversation

import (
	"errors"
	"testing"

	"github.com/codewandler/llmadapter/unified"
	"github.com/stretchr/testify/require"
)

func TestTurnFragmentPayloadsAfterCompletion(t *testing.T) {
	fragment := NewTurnFragment()
	fragment.AddRequestMessages(unified.Message{
		Role:    unified.RoleUser,
		Content: []unified.ContentPart{unified.TextPart{Text: "hello"}},
	})
	fragment.SetAssistantMessage(unified.Message{
		Role:    unified.RoleAssistant,
		Content: []unified.ContentPart{unified.TextPart{Text: "hi"}},
	})
	fragment.SetUsage(unified.Usage{
		Tokens: unified.TokenItems{{Kind: unified.TokenKindInputNew, Count: 1}},
	})
	fragment.AddContinuation(NewProviderContinuation(
		ProviderIdentity{ProviderName: "openrouter", APIKind: "responses", NativeModel: "model"},
		"resp_123",
		unified.Extensions{},
	))
	fragment.Complete(unified.FinishReasonStop)

	payloads, err := fragment.Payloads()
	require.NoError(t, err)
	require.Len(t, payloads, 2)

	tree := NewTree()
	_, err = tree.AppendMany(MainBranch, payloads...)
	require.NoError(t, err)
	messages, err := ProjectMessages(tree, MainBranch)
	require.NoError(t, err)
	require.Len(t, messages, 2)
	require.Equal(t, unified.RoleUser, messages[0].Role)
	require.Equal(t, unified.RoleAssistant, messages[1].Role)

	continuation, ok, err := ContinuationAtHead(tree, MainBranch, ProviderIdentity{ProviderName: "openrouter"})
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "resp_123", continuation.ResponseID)
}

func TestTurnFragmentRejectsIncompleteOrFailedFragments(t *testing.T) {
	fragment := NewTurnFragment()
	fragment.AddRequestMessages(unified.Message{Role: unified.RoleUser})

	_, err := fragment.Payloads()
	require.Error(t, err)

	fragment.Fail(errors.New("stream failed"))
	_, err = fragment.Payloads()
	require.Error(t, err)
}
