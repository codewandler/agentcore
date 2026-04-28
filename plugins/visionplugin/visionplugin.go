// Package visionplugin bundles the vision tool into a single [app.Plugin]
// implementation. It handles client construction from environment variables
// or explicit options, delegating the actual tool logic to [tools/vision].
package visionplugin

import (
	"github.com/codewandler/agentsdk/tool"
	"github.com/codewandler/agentsdk/tools/vision"
	"github.com/codewandler/llmadapter/unified"
)

// Option configures a Plugin.
type Option func(*Plugin)

// Plugin bundles the vision tool behind the app.Plugin interface.
type Plugin struct {
	client unified.Client
}

// New creates a vision plugin. Without options it auto-detects the OpenRouter
// API key from VISION_OPENROUTER_API_KEY or OPENROUTER_API_KEY.
func New(opts ...Option) *Plugin {
	p := &Plugin{}
	for _, opt := range opts {
		if opt != nil {
			opt(p)
		}
	}
	if p.client == nil {
		p.client = vision.ClientFromEnv()
	}
	return p
}

// WithClient sets a pre-built unified.Client for vision requests.
func WithClient(client unified.Client) Option {
	return func(p *Plugin) { p.client = client }
}

// WithAPIKey sets the OpenRouter API key explicitly instead of reading from
// environment variables.
func WithAPIKey(apiKey string) Option {
	return func(p *Plugin) {
		if apiKey != "" {
			p.client = newClient(apiKey)
		}
	}
}

// Name returns the plugin identity.
func (p *Plugin) Name() string { return "vision" }

// Tools returns the vision tool.
func (p *Plugin) Tools() []tool.Tool {
	return vision.Tools(p.client)
}

// newClient creates an OpenRouter chat completions client. Package-level var
// so tests can replace it.
var newClient = vision.ClientFromKey
