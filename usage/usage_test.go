package usage

import (
	"testing"

	"github.com/codewandler/llmadapter/unified"
	"github.com/stretchr/testify/require"
)

func TestTrackerAggregateExcludesEstimates(t *testing.T) {
	tr := NewTracker()
	tr.Record(Record{
		IsEstimate: true,
		Usage: unified.Usage{Tokens: unified.TokenItems{
			{Kind: unified.TokenKindInputNew, Count: 100},
		}},
	})
	tr.Record(FromUnified(unified.Usage{
		Tokens: unified.TokenItems{
			{Kind: unified.TokenKindInputNew, Count: 10},
			{Kind: unified.TokenKindOutput, Count: 5},
		},
		Costs: unified.CostItems{{Kind: unified.CostKindInput, Amount: 0.01}},
	}, Dims{Provider: "test", Model: "m"}))

	agg := tr.Aggregate()
	require.Equal(t, 10, agg.Usage.Tokens.Count(unified.TokenKindInputNew))
	require.Equal(t, 5, agg.Usage.Tokens.Count(unified.TokenKindOutput))
	require.Equal(t, 0.01, agg.Usage.Costs.ByKind(unified.CostKindInput))
}

func TestTrackerDrift(t *testing.T) {
	tr := NewTracker()
	dims := Dims{RequestID: "req_1"}
	tr.Record(Record{
		Dims:       dims,
		IsEstimate: true,
		Usage:      unified.Usage{Tokens: unified.TokenItems{{Kind: unified.TokenKindInputNew, Count: 50}}},
	})
	tr.Record(FromUnified(unified.Usage{
		Tokens: unified.TokenItems{{Kind: unified.TokenKindInputNew, Count: 75}},
	}, dims))

	drift, ok := tr.Drift("req_1")
	require.True(t, ok)
	require.Equal(t, 25, drift.InputDelta)
	require.Equal(t, 50.0, drift.InputPct)
}

func TestCostCalculator(t *testing.T) {
	tr := NewTracker(WithCostCalculator(CostCalculatorFunc(func(provider, model string, tokens unified.TokenItems) (unified.CostItems, bool) {
		require.Equal(t, "p", provider)
		require.Equal(t, "m", model)
		return unified.CostItems{{Kind: unified.CostKindOutput, Amount: 0.02}}, true
	})))
	tr.Record(FromUnified(unified.Usage{
		Tokens: unified.TokenItems{{Kind: unified.TokenKindOutput, Count: 10}},
	}, Dims{Provider: "p", Model: "m"}))

	agg := tr.Aggregate()
	require.Equal(t, 0.02, agg.Usage.Costs.ByKind(unified.CostKindOutput))
}
