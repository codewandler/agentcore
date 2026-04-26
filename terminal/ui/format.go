package ui

import (
	"fmt"
	"strings"

	"github.com/codewandler/agentsdk/usage"
	"github.com/codewandler/llmadapter/unified"
)

// CompactCount formats large counts for compact terminal display.
func CompactCount(n int) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	if n < 100_000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return fmt.Sprintf("%.0fk", float64(n)/1000)
}

// FormatCost formats a dollar cost with adaptive precision.
func FormatCost(cost float64) string {
	if cost == 0 {
		return ""
	}
	switch {
	case cost < 0.0001:
		return fmt.Sprintf("$%.6f", cost)
	case cost < 1.0:
		return fmt.Sprintf("$%.4f", cost)
	default:
		return fmt.Sprintf("$%.2f", cost)
	}
}

// FormatUsageParts builds a compact usage summary string.
func FormatUsageParts(rec usage.Record) string {
	var parts []string
	totalIn := rec.Usage.Tokens.InputTotal()
	cacheRead := rec.Usage.Tokens.Count(unified.TokenKindInputCacheRead)
	cacheWrite := rec.Usage.Tokens.Count(unified.TokenKindInputCacheWrite)
	nonCache := rec.Usage.Tokens.Count(unified.TokenKindInputNew)
	if totalIn > 0 {
		if cacheRead > 0 || cacheWrite > 0 {
			var cacheParts []string
			if cacheRead > 0 {
				hitRate := float64(cacheRead) * 100.0 / float64(totalIn)
				cacheParts = append(cacheParts, fmt.Sprintf("cache_r: %s %.1f%%", CompactCount(cacheRead), hitRate))
			}
			if cacheWrite > 0 {
				cacheParts = append(cacheParts, fmt.Sprintf("cache_w: %s", CompactCount(cacheWrite)))
			}
			if nonCache > 0 {
				cacheParts = append(cacheParts, fmt.Sprintf("new: %s", CompactCount(nonCache)))
			}
			parts = append(parts, fmt.Sprintf("in: %s (%s)", CompactCount(totalIn), strings.Join(cacheParts, "  ")))
		} else {
			parts = append(parts, fmt.Sprintf("in: %s", CompactCount(totalIn)))
		}
	}
	if output := rec.Usage.Tokens.Count(unified.TokenKindOutput); output > 0 {
		parts = append(parts, fmt.Sprintf("out: %s", CompactCount(output)))
	}
	if reasoning := rec.Usage.Tokens.Count(unified.TokenKindOutputReasoning); reasoning > 0 {
		parts = append(parts, fmt.Sprintf("reason: %s", CompactCount(reasoning)))
	}
	if cost := FormatCost(rec.Usage.Costs.Total()); cost != "" {
		parts = append(parts, fmt.Sprintf("cost: %s", cost))
	}
	return strings.Join(parts, "  ")
}
