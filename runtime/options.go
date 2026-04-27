package runtime

import (
	"context"
	"time"

	"github.com/codewandler/agentsdk/agentcontext"
	"github.com/codewandler/agentsdk/capability"
	"github.com/codewandler/agentsdk/conversation"
	"github.com/codewandler/agentsdk/runner"
	"github.com/codewandler/agentsdk/tool"
	"github.com/codewandler/llmadapter/unified"
)

type Option func(*Engine)

func WithSession(session *conversation.Session) Option {
	return func(e *Engine) { e.session = session }
}

func WithSessionOptions(opts ...conversation.Option) Option {
	return func(e *Engine) { e.sessionOptions = append(e.sessionOptions, opts...) }
}

func WithModel(model string) Option {
	return func(e *Engine) {
		e.request.Model = model
		e.sessionOptions = append(e.sessionOptions, conversation.WithModel(model))
	}
}

func WithMaxOutputTokens(max int) Option {
	return func(e *Engine) {
		e.request.MaxOutputTokens = &max
		e.sessionOptions = append(e.sessionOptions, conversation.WithMaxOutputTokens(max))
	}
}

func WithTemperature(value float64) Option {
	return func(e *Engine) {
		e.request.Temperature = &value
		e.sessionOptions = append(e.sessionOptions, conversation.WithTemperature(value))
	}
}

func WithSystem(text string) Option {
	return func(e *Engine) { e.sessionOptions = append(e.sessionOptions, conversation.WithSystem(text)) }
}

func WithReasoning(reasoning unified.ReasoningConfig) Option {
	return func(e *Engine) {
		e.request.Reasoning = &reasoning
		e.sessionOptions = append(e.sessionOptions, conversation.WithReasoning(reasoning))
	}
}

func WithTools(tools []tool.Tool) Option {
	return func(e *Engine) {
		e.tools = append([]tool.Tool(nil), tools...)
		unifiedTools := tool.UnifiedToolsFrom(tools)
		e.request.Tools = unifiedTools
		e.sessionOptions = append(e.sessionOptions, conversation.WithTools(unifiedTools))
	}
}

func WithToolChoice(choice unified.ToolChoice) Option {
	return func(e *Engine) {
		e.request.ToolChoice = &choice
		e.sessionOptions = append(e.sessionOptions, conversation.WithToolChoice(choice))
	}
}

func WithCachePolicy(policy unified.CachePolicy) Option {
	return func(e *Engine) {
		e.request.CachePolicy = policy
		e.sessionOptions = append(e.sessionOptions, conversation.WithCachePolicy(policy))
	}
}

func WithCacheKey(key string) Option {
	return func(e *Engine) {
		e.request.CacheKey = key
		e.sessionOptions = append(e.sessionOptions, conversation.WithCacheKey(key))
	}
}

func WithCacheTTL(ttl string) Option {
	return func(e *Engine) {
		e.request.CacheTTL = ttl
		e.sessionOptions = append(e.sessionOptions, conversation.WithCacheTTL(ttl))
	}
}

func WithProjectionPolicy(policy conversation.ProjectionPolicy) Option {
	return func(e *Engine) {
		e.sessionOptions = append(e.sessionOptions, conversation.WithProjectionPolicy(policy))
	}
}

func WithStream(stream bool) Option {
	return func(e *Engine) { e.request.Stream = stream }
}

func WithRequestDefaults(req conversation.Request) Option {
	return func(e *Engine) { e.request = cloneRequest(req) }
}

func WithMaxSteps(max int) Option {
	return func(e *Engine) {
		if max > 0 {
			e.maxSteps = max
		}
	}
}

func WithToolCtx(ctx tool.Ctx) Option {
	return func(e *Engine) { e.toolCtx = ctx }
}

func WithToolContextFactory(factory func(context.Context) tool.Ctx) Option {
	return func(e *Engine) { e.toolCtxFactory = factory }
}

func WithToolTimeout(timeout time.Duration) Option {
	return func(e *Engine) {
		if timeout > 0 {
			e.toolTimeout = timeout
		}
	}
}

func WithToolExecutor(executor runner.ToolExecutor) Option {
	return func(e *Engine) { e.toolExecutor = executor }
}

func WithProviderIdentity(identity conversation.ProviderIdentity) Option {
	return func(e *Engine) { e.providerIdentity = identity }
}

func WithRequestPreparer(preparer runner.RequestPreparer) Option {
	return func(e *Engine) { e.requestPreparer = preparer }
}

func WithThreadRuntime(runtime *ThreadRuntime) Option {
	return func(e *Engine) { e.threadRuntime = runtime }
}

// WithThreadContextManager supplies the context manager used when a thread
// runtime is built by the engine options.
func WithThreadContextManager(manager *agentcontext.Manager) Option {
	return func(e *Engine) { e.threadContexts = manager }
}

// WithContextProviders registers additional context providers on the thread
// runtime context manager. When WithThreadRuntime is supplied, providers are
// registered on that runtime's manager during Engine construction.
func WithContextProviders(providers ...agentcontext.Provider) Option {
	return func(e *Engine) {
		e.contextProviders = append(e.contextProviders, providers...)
	}
}

func WithCapabilities(specs ...capability.AttachSpec) Option {
	return func(e *Engine) {
		e.capabilitySpecs = append(e.capabilitySpecs, specs...)
	}
}

func WithEventHandler(handler runner.EventHandler) Option {
	return func(e *Engine) { e.onEvent = handler }
}

type TurnOption func(*TurnConfig)

type TurnConfig struct {
	Request          conversation.Request
	MaxSteps         int
	Tools            []tool.Tool
	ToolCtx          tool.Ctx
	ToolTimeout      time.Duration
	ToolExecutor     runner.ToolExecutor
	ToolCtxFactory   func(context.Context) tool.Ctx
	ProviderIdentity conversation.ProviderIdentity
	RequestPreparer  runner.RequestPreparer
	OnEvent          runner.EventHandler
}

func WithTurnRequest(req conversation.Request) TurnOption {
	return func(c *TurnConfig) { c.Request = req }
}

func WithTurnTools(tools []tool.Tool) TurnOption {
	return func(c *TurnConfig) {
		c.Tools = append([]tool.Tool(nil), tools...)
		c.Request.Tools = tool.UnifiedToolsFrom(tools)
	}
}

func WithTurnToolCtx(ctx tool.Ctx) TurnOption {
	return func(c *TurnConfig) { c.ToolCtx = ctx }
}

func WithTurnToolContextFactory(factory func(context.Context) tool.Ctx) TurnOption {
	return func(c *TurnConfig) { c.ToolCtxFactory = factory }
}

func WithTurnEventHandler(handler runner.EventHandler) TurnOption {
	return func(c *TurnConfig) { c.OnEvent = handler }
}

func WithTurnProviderIdentity(identity conversation.ProviderIdentity) TurnOption {
	return func(c *TurnConfig) { c.ProviderIdentity = identity }
}

func WithTurnRequestPreparer(preparer runner.RequestPreparer) TurnOption {
	return func(c *TurnConfig) { c.RequestPreparer = preparer }
}

func WithTurnMaxSteps(max int) TurnOption {
	return func(c *TurnConfig) {
		if max > 0 {
			c.MaxSteps = max
		}
	}
}
