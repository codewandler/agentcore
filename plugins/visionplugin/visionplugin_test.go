package visionplugin

import (
	"context"
	"testing"

	"github.com/codewandler/agentsdk/app"
	"github.com/codewandler/llmadapter/unified"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Compile-time interface assertions.
var (
	_ app.Plugin      = (*Plugin)(nil)
	_ app.ToolsPlugin = (*Plugin)(nil)
)

func TestPluginName(t *testing.T) {
	p := New(WithClient(nopClient{}))
	require.Equal(t, "vision", p.Name())
}

func TestPluginToolsReturnsVisionTool(t *testing.T) {
	p := New(WithClient(nopClient{}))
	tools := p.Tools()
	require.Len(t, tools, 1)
	assert.Equal(t, "vision", tools[0].Name())
}

func TestPluginWithClient(t *testing.T) {
	mock := nopClient{}
	p := New(WithClient(mock))
	require.NotNil(t, p.client)
	tools := p.Tools()
	require.Len(t, tools, 1)
	// Tool should be functional (not the "not configured" stub).
	assert.Contains(t, tools[0].Description(), "Analyze images")
}

func TestPluginWithAPIKey(t *testing.T) {
	// Replace the newClient factory for testing so we don't need a real
	// OpenRouter key.
	origFactory := newClient
	defer func() { newClient = origFactory }()

	var capturedKey string
	newClient = func(apiKey string) unified.Client {
		capturedKey = apiKey
		return nopClient{}
	}

	p := New(WithAPIKey("test-key-123"))
	require.NotNil(t, p.client)
	assert.Equal(t, "test-key-123", capturedKey)
}

func TestPluginWithAPIKey_Empty(t *testing.T) {
	// Empty API key should not set a client.
	p := &Plugin{}
	opt := WithAPIKey("")
	opt(p)
	assert.Nil(t, p.client)
}

func TestPluginNoClient_ReportsConfigError(t *testing.T) {
	// When no client is available (no env vars, no options), the tool
	// should still be returned but report a configuration error.
	origFactory := newClient
	defer func() { newClient = origFactory }()
	newClient = func(string) unified.Client { return nil }

	p := &Plugin{} // no client set
	tools := p.Tools()
	require.Len(t, tools, 1)
	assert.Contains(t, tools[0].Description(), "unavailable")
}

func TestPluginNilOption(t *testing.T) {
	// Nil options should not panic.
	origFactory := newClient
	defer func() { newClient = origFactory }()
	newClient = func(string) unified.Client { return nil }

	require.NotPanics(t, func() {
		_ = New(nil, WithClient(nopClient{}), nil)
	})
}

// nopClient is a minimal unified.Client for testing.
type nopClient struct{}

func (nopClient) Request(_ context.Context, _ unified.Request) (<-chan unified.Event, error) {
	ch := make(chan unified.Event, 1)
	ch <- unified.CompletedEvent{FinishReason: unified.FinishReasonStop}
	close(ch)
	return ch, nil
}
