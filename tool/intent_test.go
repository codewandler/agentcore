package tool

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

// ── ExtractIntent: no IntentProvider ──────────────────────────────────────────

func TestExtractIntent_OpaqueWhenNoProvider(t *testing.T) {
	base := &fakeTool{name: "mystery"}

	intent := ExtractIntent(base, testCtx(), json.RawMessage(`{}`))

	require.Equal(t, "mystery", intent.Tool)
	require.Equal(t, "unknown", intent.ToolClass)
	require.True(t, intent.Opaque)
	require.Equal(t, "low", intent.Confidence)
}

// ── ExtractIntent: with IntentProvider ────────────────────────────────────────

func TestExtractIntent_UsesIntentProvider(t *testing.T) {
	base := &intentTool{
		fakeTool: fakeTool{name: "file_read"},
		intent: Intent{
			Tool:       "file_read",
			ToolClass:  "filesystem_read",
			Confidence: "high",
			Operations: []IntentOperation{{
				Resource:  IntentResource{Category: "file", Value: "/tmp/x", Locality: "workspace"},
				Operation: "read",
				Certain:   true,
			}},
			Behaviors: []string{"filesystem_read"},
		},
	}

	intent := ExtractIntent(base, testCtx(), json.RawMessage(`{}`))

	require.Equal(t, "file_read", intent.Tool)
	require.Equal(t, "filesystem_read", intent.ToolClass)
	require.Equal(t, "high", intent.Confidence)
	require.False(t, intent.Opaque)
	require.Len(t, intent.Operations, 1)
	require.Equal(t, "read", intent.Operations[0].Operation)
}

// ── ExtractIntent: DeclareIntent error → opaque fallback ─────────────────────

func TestExtractIntent_FallbackOnDeclareIntentError(t *testing.T) {
	base := &intentTool{
		fakeTool: fakeTool{name: "broken"},
		err:      errors.New("parse failed"),
	}

	intent := ExtractIntent(base, testCtx(), json.RawMessage(`{}`))

	require.Equal(t, "broken", intent.Tool)
	require.True(t, intent.Opaque)
	require.Equal(t, "low", intent.Confidence)
}

// ── ExtractIntent: middleware chain amends intent ─────────────────────────────

func TestExtractIntent_MiddlewareAmendsIntent(t *testing.T) {
	base := &intentTool{
		fakeTool: fakeTool{name: "file_read"},
		intent: Intent{
			Tool:       "file_read",
			ToolClass:  "filesystem_read",
			Confidence: "high",
			Operations: []IntentOperation{{
				Resource:  IntentResource{Category: "file", Value: "/tmp/x", Locality: "unknown"},
				Operation: "read",
				Certain:   true,
			}},
		},
	}

	// Middleware that upgrades "unknown" locality to "workspace".
	m := HooksMiddleware(&localityUpgradeHooks{})
	wrapped := Apply(base, m)

	intent := ExtractIntent(wrapped, testCtx(), json.RawMessage(`{}`))

	require.Equal(t, "workspace", intent.Operations[0].Resource.Locality)
}

func TestExtractIntent_MiddlewareChainOrder(t *testing.T) {
	base := &intentTool{
		fakeTool: fakeTool{name: "test"},
		intent: Intent{
			Tool:       "test",
			Confidence: "high",
			Behaviors:  []string{"original"},
		},
	}

	// Inner middleware appends "inner".
	inner := HooksMiddleware(&appendBehaviorHooks{behavior: "inner"})
	// Outer middleware appends "outer".
	outer := HooksMiddleware(&appendBehaviorHooks{behavior: "outer"})

	wrapped := Apply(base, inner, outer)
	intent := ExtractIntent(wrapped, testCtx(), json.RawMessage(`{}`))

	// Inside-out: inner runs first, then outer.
	require.Equal(t, []string{"original", "inner", "outer"}, intent.Behaviors)
}

// ── ExtractIntent: unwrapped tool with no middleware ─────────────────────────

func TestExtractIntent_NoMiddleware(t *testing.T) {
	base := &intentTool{
		fakeTool: fakeTool{name: "simple"},
		intent: Intent{
			Tool:       "simple",
			ToolClass:  "filesystem_read",
			Confidence: "high",
		},
	}

	intent := ExtractIntent(base, testCtx(), json.RawMessage(`{}`))

	require.Equal(t, "simple", intent.Tool)
	require.Equal(t, "filesystem_read", intent.ToolClass)
}

// ── TypedTool.DeclareIntent ──────────────────────────────────────────────────

func TestTypedTool_DeclareIntent_WithOption(t *testing.T) {
	type params struct {
		Path string `json:"path"`
	}

	tl := New("file_read", "Read a file",
		func(ctx Ctx, p params) (Result, error) {
			return Text("ok"), nil
		},
		WithDeclareIntent(func(ctx Ctx, p params) (Intent, error) {
			return Intent{
				Tool:       "file_read",
				ToolClass:  "filesystem_read",
				Confidence: "high",
				Operations: []IntentOperation{{
					Resource:  IntentResource{Category: "file", Value: p.Path, Locality: "workspace"},
					Operation: "read",
					Certain:   true,
				}},
				Behaviors: []string{"filesystem_read"},
			}, nil
		}),
	)

	intent, err := tl.DeclareIntent(testCtx(), json.RawMessage(`{"path":"/tmp/test.go"}`))
	require.NoError(t, err)
	require.Equal(t, "file_read", intent.Tool)
	require.Equal(t, "filesystem_read", intent.ToolClass)
	require.Equal(t, "high", intent.Confidence)
	require.Len(t, intent.Operations, 1)
	require.Equal(t, "/tmp/test.go", intent.Operations[0].Resource.Value)
}

func TestTypedTool_DeclareIntent_WithoutOption(t *testing.T) {
	type params struct {
		Query string `json:"query"`
	}

	tl := New("search", "Search things",
		func(ctx Ctx, p params) (Result, error) {
			return Text("ok"), nil
		},
	)

	intent, err := tl.DeclareIntent(testCtx(), json.RawMessage(`{"query":"test"}`))
	require.NoError(t, err)
	require.Equal(t, "search", intent.Tool)
	require.Equal(t, "unknown", intent.ToolClass)
	require.True(t, intent.Opaque)
	require.Equal(t, "low", intent.Confidence)
}

func TestTypedTool_DeclareIntent_BadJSON(t *testing.T) {
	type params struct {
		Path string `json:"path"`
	}

	tl := New("file_read", "Read a file",
		func(ctx Ctx, p params) (Result, error) {
			return Text("ok"), nil
		},
		WithDeclareIntent(func(ctx Ctx, p params) (Intent, error) {
			return Intent{Tool: "file_read", ToolClass: "filesystem_read", Confidence: "high"}, nil
		}),
	)

	// Bad JSON → graceful fallback to opaque.
	intent, err := tl.DeclareIntent(testCtx(), json.RawMessage(`not json`))
	require.NoError(t, err)
	require.True(t, intent.Opaque)
}

func TestTypedTool_DeclareIntent_NullInput(t *testing.T) {
	type params struct {
		Path string `json:"path"`
	}

	called := false
	tl := New("test", "test",
		func(ctx Ctx, p params) (Result, error) {
			return Text("ok"), nil
		},
		WithDeclareIntent(func(ctx Ctx, p params) (Intent, error) {
			called = true
			require.Equal(t, "", p.Path) // zero value
			return Intent{Tool: "test", Confidence: "high"}, nil
		}),
	)

	_, err := tl.DeclareIntent(testCtx(), json.RawMessage(`null`))
	require.NoError(t, err)
	require.True(t, called)
}

// ── ExtractIntent with TypedTool ─────────────────────────────────────────────

func TestExtractIntent_WithTypedTool(t *testing.T) {
	type params struct {
		Path string `json:"path"`
	}

	tl := New("file_read", "Read a file",
		func(ctx Ctx, p params) (Result, error) {
			return Text("ok"), nil
		},
		WithDeclareIntent(func(ctx Ctx, p params) (Intent, error) {
			return Intent{
				Tool:       "file_read",
				ToolClass:  "filesystem_read",
				Confidence: "high",
				Operations: []IntentOperation{{
					Resource:  IntentResource{Category: "file", Value: p.Path, Locality: "workspace"},
					Operation: "read",
					Certain:   true,
				}},
			}, nil
		}),
	)

	intent := ExtractIntent(tl, testCtx(), json.RawMessage(`{"path":"src/main.go"}`))

	require.Equal(t, "file_read", intent.Tool)
	require.Equal(t, "filesystem_read", intent.ToolClass)
	require.Equal(t, "src/main.go", intent.Operations[0].Resource.Value)
}

// ── Test hook implementations ────────────────────────────────────────────────

// intentTool is a fakeTool that implements IntentProvider.
type intentTool struct {
	fakeTool
	intent Intent
	err    error
}

func (t *intentTool) DeclareIntent(_ Ctx, _ json.RawMessage) (Intent, error) {
	return t.intent, t.err
}

// localityUpgradeHooks upgrades "unknown" locality to "workspace".
type localityUpgradeHooks struct {
	HooksBase
}

func (h *localityUpgradeHooks) OnIntent(_ Ctx, _ Tool, intent Intent, _ CallState) Intent {
	for i := range intent.Operations {
		if intent.Operations[i].Resource.Locality == "unknown" {
			intent.Operations[i].Resource.Locality = "workspace"
		}
	}
	return intent
}

// appendBehaviorHooks appends a behavior string to the intent.
type appendBehaviorHooks struct {
	HooksBase
	behavior string
}

func (h *appendBehaviorHooks) OnIntent(_ Ctx, _ Tool, intent Intent, _ CallState) Intent {
	intent.Behaviors = append(intent.Behaviors, h.behavior)
	return intent
}
