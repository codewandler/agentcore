package usage

import (
	"testing"

	"github.com/codewandler/agentsdk/conversation"
	"github.com/codewandler/agentsdk/runner"
	"github.com/codewandler/llmadapter/unified"
	"github.com/stretchr/testify/require"
)

func TestProviderAndModelUsesIdentityFirst(t *testing.T) {
	provider, model := ProviderAndModel(
		conversation.ProviderIdentity{ProviderName: "claude", NativeModel: "claude-sonnet-4-6"},
		RouteState{Provider: "openai", Model: "gpt-5.4"},
		"fallback/model",
	)

	require.Equal(t, "claude", provider)
	require.Equal(t, "claude-sonnet-4-6", model)
}

func TestProviderAndModelFallsBackToRouteState(t *testing.T) {
	provider, model := ProviderAndModel(conversation.ProviderIdentity{}, RouteState{Provider: "codex", Model: "gpt-5.4"}, "")

	require.Equal(t, "codex", provider)
	require.Equal(t, "gpt-5.4", model)
}

func TestProviderAndModelSplitsProviderQualifiedFallback(t *testing.T) {
	provider, model := ProviderAndModel(conversation.ProviderIdentity{}, RouteState{}, "openai/gpt-4.1")

	require.Equal(t, "openai", provider)
	require.Equal(t, "gpt-4.1", model)
}

func TestProviderAndModelStripsProviderPrefix(t *testing.T) {
	provider, model := ProviderAndModel(conversation.ProviderIdentity{ProviderName: "openai", NativeModel: "openai/gpt-4.1"}, RouteState{}, "")

	require.Equal(t, "openai", provider)
	require.Equal(t, "gpt-4.1", model)
}

func TestFromRunnerEventBuildsTransportRecord(t *testing.T) {
	rec := FromRunnerEvent(runner.UsageEvent{
		Model: "fallback/model",
		Usage: unified.Usage{Tokens: unified.TokenItems{
			{Kind: unified.TokenKindInputNew, Count: 7},
		}},
	}, RunnerEventOptions{
		TurnID:    "3",
		SessionID: "sess",
		RouteState: RouteState{
			Provider: "route",
			Model:    "route-model",
		},
		Labels: map[string]string{"app": "test"},
	})

	require.Equal(t, "route", rec.Dims.Provider)
	require.Equal(t, "route-model", rec.Dims.Model)
	require.Equal(t, "3", rec.Dims.TurnID)
	require.Equal(t, "sess", rec.Dims.SessionID)
	require.Equal(t, "test", rec.Dims.Labels["app"])
	require.Equal(t, DefaultTransportSource, rec.Source)
	require.Equal(t, 7, rec.Usage.Tokens.Count(unified.TokenKindInputNew))
}

func TestTrackerAggregateTurn(t *testing.T) {
	tr := NewTracker()
	tr.Record(Record{
		Dims:       Dims{TurnID: "1"},
		IsEstimate: true,
		Usage:      unified.Usage{Tokens: unified.TokenItems{{Kind: unified.TokenKindInputNew, Count: 100}}},
	})
	tr.Record(FromUnified(unified.Usage{
		Tokens: unified.TokenItems{{Kind: unified.TokenKindOutput, Count: 5}},
	}, Dims{TurnID: "1"}))
	tr.Record(FromUnified(unified.Usage{
		Tokens: unified.TokenItems{{Kind: unified.TokenKindOutput, Count: 9}},
	}, Dims{TurnID: "2"}))

	agg := tr.AggregateTurn("1")

	require.Equal(t, 0, agg.Usage.Tokens.Count(unified.TokenKindInputNew))
	require.Equal(t, 5, agg.Usage.Tokens.Count(unified.TokenKindOutput))
}
