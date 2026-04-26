# codewandler/agentsdk

Portable tool definitions, conversation/runtime helpers, markdown utilities, and instruction file loading for LLM agents.

**agentsdk** is a small foundation for agentic CLIs and applications. It owns reusable agent mechanics such as tools, conversation state, runtime turns, usage accounting, and markdown parsing, while consumers keep product policy, terminal rendering, prompts, and storage locations.

## Features

- **Tool system**: Define and execute LLM agent tools with schema validation.
- **Standard tools**: Filesystem, shell, git, web, notifications, todo, and tool activation management.
- **Runtime facade**: Run model/tool turns over `llmadapter/unified.Client`.
- **Terminal app shell**: Build Cobra-based terminal agents and run filesystem
  agent bundles with `agentsdk run`.
- **Resource discovery**: Load `.agents` and `.claude` agents, commands, and
  skills from local directories or declarative git sources.
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
- Ship slim filesystem-described agents without writing a full Go app.

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

## CLI Resource Bundles

The built-in CLI can run an agent described by files on disk:

```bash
go run ./cmd/agentsdk run [path] [task]
```

`path` defaults to the current working directory. When no explicit agent is
found, agentsdk uses a small built-in general-purpose terminal agent so
`agentsdk run` still opens a usable REPL.

Resource roots can be shaped like either project compatibility directories or
plugin roots:

```text
.agents/
  agents/
  commands/
  skills/

.claude/
  agents/
  commands/
  skills/

plugin-root/
  agents/
  commands/
  skills/
```

Agent files live in `agents/*.md` and use YAML frontmatter plus a Markdown
system prompt body:

```markdown
---
name: coder
description: General coding agent
tools: [bash, file_read]
skills: [go]
commands: [review]
---
You are a concise coding agent.
```

Command files live in `commands/*.md`; their frontmatter is parsed into slash
command metadata and their body is used as a prompt template. Skills use the
`SKILL.md` directory format under `skills/<name>/SKILL.md`.

To inspect what agentsdk can load without running an agent:

```bash
go run ./cmd/agentsdk discover [path]
go run ./cmd/agentsdk discover --local [path]
```

`discover` is broad by default: it includes the target path, global user
resources, manifest-declared git sources, and disabled suggestions for known
external files such as `AGENTS.md` or `Taskfile.yaml`. `--local` limits
inspection to the target path and disables global and remote sources.

Generic `agentsdk run` does not include global user resources by default. Pass
`--include-global` to include `~/.agents` and `~/.claude`:

```bash
go run ./cmd/agentsdk run . --include-global
```

## App Manifests

A directory can contain `app.manifest.json` or `agentsdk.app.json` to declare
the app's default agent, discovery policy, and source list:

```json
{
  "default_agent": "coder",
  "discovery": {
    "include_global_user_resources": false,
    "include_external_ecosystems": false,
    "allow_remote": false,
    "trust_store_dir": ".agentsdk"
  },
  "sources": [
    ".agents",
    "file:///absolute/path/to/plugin",
    "git+https://github.com/codewandler/agentplugins.git#main"
  ]
}
```

Source strings are URL-like:

- Bare paths are resolved relative to the manifest directory.
- `file://...` points at an explicit local directory.
- `git+https://...` and `git+ssh://...` materialize a git repository/ref under
  `<workspace>/.agentsdk/cache/git/...` and then load declarative resources
  from it.

Remote sources are declarative only; repository code is not executed just by
loading it.

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
