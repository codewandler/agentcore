# codewandler/agentsdk

Portable tool definitions, conversation/runtime helpers, markdown utilities, and instruction file loading for LLM agents.

**agentsdk** is a small foundation for agentic CLIs and applications. It owns reusable agent mechanics such as tools, conversation state, runtime turns, usage accounting, and markdown parsing, while consumers keep product policy, terminal rendering, prompts, and storage locations.

## Features

- **Tool system**: Define and execute LLM agent tools with schema validation.
- **Standard tools**: Filesystem, shell, git, web, notifications, todo, and tool activation management.
- **Runtime facade**: Run model/tool turns over `llmadapter/unified.Client`.
- **Conversation state**: Session IDs, conversation IDs, branches, replay projection, JSONL persistence, and provider continuation metadata.
- **Usage tracking**: Aggregate `llmadapter/unified` token and cost records with runner usage helpers.
- **Markdown + frontmatter**: Parse and load structured instruction files.
- **Instruction loading**: AGENTS.md and CLAUDE.md pattern support.

## Use Cases

- Build custom agent tools without a full application framework.
- Build CLI/UI agents while keeping terminal rendering and product policy in the consumer.
- Share a tool vocabulary across projects.
- Persist and resume conversation sessions.
- Load configuration from markdown instruction files.

## Recommended Runtime Stack

Use `runtime` as the high-level turn loop, `tools/standard` for the default tool bundle, and `llmadapter` auto mux for provider selection.

```go
import (
    "context"

    "github.com/codewandler/agentsdk/runner"
    "github.com/codewandler/agentsdk/runtime"
    "github.com/codewandler/agentsdk/tool"
    "github.com/codewandler/agentsdk/tools/standard"
    "github.com/codewandler/llmadapter/adapt"
    "github.com/codewandler/llmadapter/unified"
)

model := "codex/gpt-5.4"
sourceAPI := adapt.ApiOpenAIResponses

auto, err := runtime.AutoMuxClient(model, sourceAPI, nil)
if err != nil {
    return err
}
identity, _, _ := runtime.RouteIdentity(auto, sourceAPI, model)

toolset := standard.DefaultToolset()
agent, err := runtime.New(auto.Client,
    runtime.WithProviderIdentity(identity),
    runtime.WithModel(model),
    runtime.WithSystem("You are a concise coding assistant."),
    runtime.WithTools(toolset.ActiveTools()),
    runtime.WithToolChoice(unified.ToolChoice{Mode: unified.ToolChoiceAuto}),
    runtime.WithCachePolicy(unified.CachePolicyOn),
    runtime.WithCacheKey("session-id"),
    runtime.WithMaxSteps(8),
    runtime.WithToolContextFactory(func(ctx context.Context) tool.Ctx {
        return runtime.NewToolContext(ctx,
            runtime.WithToolWorkDir("."),
            runtime.WithToolSessionID("session-id"),
            runtime.WithToolActivation(toolset.Activation()),
        )
    }),
    runtime.WithEventHandler(func(event runner.Event) {
        // Render text/tool/usage events in your application.
    }),
)
if err != nil {
    return err
}

_, err = agent.RunTurn(context.Background(), "inspect this repo")
```

### Durable Sessions

Consumers own the session storage path policy. Use `conversation/jsonlstore` for append-only JSONL persistence and `runtime.SessionOptions` so persistent sessions receive the same defaults as the runtime agent.

```go
import (
    "github.com/codewandler/agentsdk/conversation"
    "github.com/codewandler/agentsdk/conversation/jsonlstore"
    "github.com/codewandler/agentsdk/runtime"
)

store := jsonlstore.Open("/path/to/sessions/20260425T120000Z-session-id.jsonl")
session := conversation.New(append(
    runtime.SessionOptions(
        runtime.WithSessionOptions(conversation.WithSessionID("session-id")),
        runtime.WithModel(model),
        runtime.WithTools(toolset.ActiveTools()),
        runtime.WithCachePolicy(unified.CachePolicyOn),
        runtime.WithCacheKey("session-id"),
    ),
    conversation.WithStore(store),
    conversation.WithConversationID("conv_session-id"),
)...)
```

Pass the session to the runtime with `runtime.WithSession(session)`. To resume, open the same store and call `conversation.Resume` with the same `runtime.SessionOptions(...)` defaults.

### Cache And History Rules

By default, applications should use stable per-session cache affinity:

```go
runtime.WithCachePolicy(unified.CachePolicyOn)
runtime.WithCacheKey(sessionID)
```

Provider history must remain immutable. Do not trim, compact, summarize, or otherwise rewrite projected history for cost control. For providers that support native continuation, agentsdk can send the provider continuation handle; otherwise it replays the canonical selected-branch history. Usage response data is for observability, reporting, warnings, or explicit product UX, not automatic SDK history rewriting.

## Tool Context

Use `runtime.NewToolContext` when tools need a work directory, session ID, or extra app state:

```go
toolCtx := runtime.NewToolContext(ctx,
    runtime.WithToolWorkDir(workDir),
    runtime.WithToolSessionID(sessionID),
    runtime.WithToolActivation(toolset.Activation()),
)
```

`runtime.WithToolActivation` wires the state used by `tools_list`, `tools_activate`, and `tools_deactivate`.

## Lower-Level Packages

Use these directly when `runtime.Agent` is too high level:

- `conversation`: branchable event-log sessions, request projection, JSONL storage, and provider continuations.
- `runner`: model/tool loop over `llmadapter/unified.Client` with typed UI events.
- `tool`: tool definitions, schemas, execution contracts, and unified conversion.
- `usage`: token/cost records, runner usage event conversion, aggregation, and drift helpers.
- `markdown`: markdown buffering, frontmatter, and instruction file loading.

## Web Tools

The `tools/web` package provides:

- `web_fetch` — always available when you register the web tools.
- `web_search` — available when you pass a search provider.

Agentsdk includes a Tavily provider at `github.com/codewandler/agentsdk/tools/web/tavily` and a small env-based selector:

```go
provider := web.DefaultSearchProviderFromEnv()
webTools := web.Tools(provider)
```

Environment variables:

- `TAVILY_API_KEY` — enables the default Tavily-backed web search provider.
- `WEBSEARCH_PROVIDER=tavily` — explicitly select Tavily.
- `WEBSEARCH_PROVIDER=none` — disable web search while keeping `web_fetch` available.

## Status

Under development, extracted from flai as a portable foundation. The current shape is proven through `miniagent`, which uses `runtime`, `conversation`, `usage`, `tools/standard`, and `llmadapter` auto mux helpers.

## License

MIT
