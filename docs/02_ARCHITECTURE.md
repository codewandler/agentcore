# 02 — agentsdk Architecture

## Purpose

This document describes a target architecture for agentsdk that grows out of the current implementation. It is intentionally migration-oriented: most of the required foundations already exist, but their boundaries are blurred.

The goal is not to replace the current packages with a theoretical architecture. The goal is to make existing responsibilities explicit, move them gradually to clearer homes, and add missing concepts such as workflows, actions, channels, triggers, and harness lifecycle where the current model does not yet express them.

## Current architecture summary

agentsdk currently has these major subsystems:

```text
cmd/agentsdk
  -> terminal/cli
  -> app
  -> agentdir/resource
  -> agent.Instance
  -> runtime.Engine
  -> runner.RunTurn
  -> conversation/thread/tool/capability/agentcontext
```

The important point: **agentsdk already has a runtime, app composition layer, resource discovery layer, plugin system, thread persistence model, capability model, context system, safety seed, and terminal channel.**

The future architecture should mainly clarify ownership:

- `runtime`/`runner` should stay focused on model/tool turns.
- `conversation`/`thread` should remain the durable state foundation.
- `app`/`agentdir`/`resource`/`plugins` should remain the app composition and discovery foundation.
- `terminal` should evolve into one channel over a general harness/session API.
- `agent.Instance` should shrink or become a compatibility façade around clearer session/runtime/harness concepts.
- workflows/actions should extend resource/plugin/app composition instead of introducing a separate parallel ecosystem.

## Existing bounded contexts

### Tooling

Current packages:

- `tool`
- `activation`
- `tools/*`
- `tools/standard`
- `toolmw`

Current strengths:

- `tool.Tool` is a clean model-callable interface.
- Typed tools already generate JSON schemas.
- `tool.Result` supports deterministic model-facing output and persistence.
- `tool.Intent` and `IntentProvider` provide side-effect declaration.
- Middleware can wrap tools for logging, risk gates, timeouts, and approval.
- `activation.Manager` already models active/inactive tool visibility.
- `tools/standard` provides useful batteries-included assembly.

Evolution:

- Keep `tool` as public core API.
- Reuse tool schema/result/intent machinery for workflow actions where possible.
- Keep generic local tools under `tools/`.
- Treat `tools/standard` as an app/bundle convenience, not a low-level runtime dependency.
- Move product/service/environment-specific integrations toward adapters or integration packages as they appear.

### Turn runtime

Current packages:

- `runtime`
- `runner`
- `usage`

Current strengths:

- `runner.RunTurn` is the low-level model/tool loop.
- `runtime.Engine` is the higher-level turn facade.
- `runtime.History` integrates conversation projection and provider continuations.
- `runtime.OpenThreadEngine`, `CreateThreadEngine`, and `ResumeThreadEngine` already provide durable thread-backed engines.
- `runtime.ThreadRuntime` already binds live threads to capability replay and context render replay.
- Runtime options already accept tools, context providers, capabilities, event handlers, request observers, model settings, cache policy, and tool context factories.

Evolution:

- Preserve `runtime.Engine` and `runner.RunTurn` as the low-level turn runtime.
- Remove remaining concrete tool construction/imports from runtime over time.
- Treat workflow execution as a runtime-adjacent orchestration layer above turn execution.
- Let harness/session code own multi-session lifecycle and channel/trigger dispatch.

### Conversation and thread persistence

Current packages:

- `conversation`
- `thread`
- `thread/jsonlstore`

Current strengths:

- `conversation.Tree` supports branchable history.
- `conversation.Payload` and projected `Item`s separate stored events from request projection.
- `conversation.TurnFragment` provides a transaction-like turn commit object.
- Provider continuations are modeled explicitly.
- Compaction is explicit and semantic rather than silent history rewriting.
- `thread.Event` provides append-only event persistence.
- `thread.Store` has memory and JSONL implementations.
- Thread events already store conversation, capability, context render, usage, and lifecycle records.

Evolution:

- Reuse thread events for workflow execution records rather than inventing a separate persistence mechanism.
- Add workflow/action event kinds through the existing `thread.EventDefinition` pattern.
- Keep provider history immutability and explicit compaction as design constraints.

### Capabilities

Current packages:

- `capability`
- `capabilities/planner`

Current strengths:

- Capabilities are attachable modules that provide tools, context, and state.
- Stateful capabilities apply event-sourced state events.
- `capability.Manager` and `capability.Registry` already support factory-based creation and replay.
- Planner is a working built-in stateful capability.

Evolution:

- Keep capabilities for attached stateful agent/session features.
- Do not overload capabilities to mean workflows.
- Allow workflows to require or attach capabilities when needed.
- Consider workflow execution state as either thread events or a capability only if there is a concrete need; default should be workflow-specific thread events.

### Context

Current packages:

- `agentcontext`
- `agentcontext/contextproviders`

Current strengths:

- Context providers return fragments with keys, roles, markers, authority, fingerprints, snapshots, and cache hints.
- Context manager records render state and diffs.
- Thread runtime replays context render records.
- Built-in providers already cover environment, git, time, file, command, project inventory, model info, project instructions, and skill inventory.

Evolution:

- Reuse context providers for workflow steps with selected context.
- Add step-level context selection in workflow execution rather than creating a separate context system.
- Keep context render records replayable and observable.

### Skills and commands

Current packages:

- `skill`
- `command`
- `command/markdown`
- `tools/skills`

Current strengths:

- Skills support external Agent Skills-compatible filesystem resources.
- Skill references under `references/` can be activated exactly and persisted.
- Commands support slash-command parsing, command policy, and command result semantics.
- `command.Tool` bridges agent-callable commands through `command_run`.

Evolution:

- Keep skills as instruction/reference resources, not workflows.
- Keep commands as user/app actions and prompt templates, not workflow replacement.
- Allow command actions in workflows where useful, but keep command result semantics distinct from tool/action results.

### App/resource/plugin composition

Current packages:

- `app`
- `agentdir`
- `resource`
- `plugins/*`
- `markdown`

Current strengths:

- `agentdir` resolves `.agents`, `.claude`, app manifests, global/user resources, local roots, embedded/FS roots, and declarative git sources.
- `resource.ContributionBundle` normalizes discovered contributions.
- `app.App` composes bundles, plugins, commands, tools, skill sources, context providers, middleware, and agent specs.
- `app.Plugin` already has facets for commands, agent specs, tools, skill sources, context providers, agent-context providers, and tool middleware.
- App manifests already declare default agent, discovery policy, model policy, and sources.

Evolution:

- Extend `agentdir` to discover `.agents/workflows/*.yaml`.
- Extend `resource.ContributionBundle` with workflow/action contributions.
- Extend `app.Plugin` with workflow/action facets.
- Let `app.App` register workflows/actions the same way it registers commands/tools/skills today.
- Let harness build on `app.App` instead of bypassing it.

### Terminal as current channel

Current packages:

- `terminal/cli`
- `terminal/repl`
- `terminal/ui`

Current strengths:

- `terminal/cli.Load` already resolves resources, applies CLI overrides, creates `app.App`, instantiates the default agent, configures session resume, and wires terminal UI.
- REPL and UI are functional and dogfooded.
- Runner events already provide a channel-friendly stream of text/tool/usage/step/error events.

Evolution:

- Treat terminal as the first channel.
- Extract the shared resource/app/session setup from `terminal/cli.Load` into harness or app-host code.
- Keep terminal rendering in `terminal/ui`.
- Make terminal call harness/session APIs instead of constructing the whole stack directly.

### Agent package

Current package:

- `agent`

Current strengths:

- `agent.Spec` is the resource-level blueprint.
- `agent.Instance` is a useful high-level session-backed agent object.
- Model policy, inference options, auto mux, compatibility evidence, compaction, session persistence, skill activation, context providers, standard toolset wiring, and runtime creation are already integrated.

Current problem:

`agent.Instance` is doing too much. It mixes:

- declarative spec interpretation;
- model routing/policy;
- standard toolset setup;
- skill repository/state setup;
- context provider setup;
- session/thread store setup;
- capability registry setup;
- terminal UI concerns;
- runtime engine construction;
- usage tracking and compaction.

Evolution:

- Keep `agent.Spec` as core public API.
- Keep `agent.Instance` initially for compatibility and as the current high-level façade.
- Gradually move host/session lifecycle concerns toward `harness`.
- Move terminal-specific behavior out toward terminal channel.
- Keep model policy/inference pieces close to agent spec/runtime construction unless a clearer boundary emerges.

## Target architecture

The target dependency direction:

```text
cmd/agentsdk
  -> terminal/channel adapters
  -> harness
  -> app/resource/plugin composition
  -> workflow execution
  -> runtime.Engine / runner.RunTurn
  -> core abstractions

channels/*
  -> harness/session/control APIs

triggers/*
  -> harness trigger sink/session APIs

workflow
  -> action registry + runtime/tool/command/context/thread abstractions

runtime
  -> conversation/thread/tool/capability/agentcontext abstractions

adapters/*
  -> core interfaces + third-party/environment systems
```

Avoid:

```text
runtime -> concrete tools
runtime -> terminal UI
core domain -> product-specific adapters
new workflow discovery -> separate from existing resource discovery
new plugins -> separate from existing app.Plugin facets
new persistence -> separate from thread events
```

## Workflow/action architecture

Workflow/action support should be an extension of existing app/resource/plugin/runtime concepts.

### Definitions

```text
Action
  name
  kind/type
  input schema
  output schema
  implementation
  intent declaration

Workflow
  name
  description
  input schema
  output schema
  steps/actions
  edges
  policy
  context selection
  metadata

Pipeline
  workflow whose DAG is a simple sequence
```

### Resource loading

Default resource location:

```text
.agents/workflows/*.yaml
```

But discovery should follow existing resource resolution rules:

- project `.agents` roots;
- compatibility `.claude` roots if desired/compatible;
- plugin roots;
- app manifest sources;
- embedded filesystems;
- declarative remote git sources, where safe.

### Plugin contribution

Extend `app.Plugin` with facets such as:

```go
type WorkflowsPlugin interface {
    Plugin
    Workflows() []workflow.Definition
}

type ActionsPlugin interface {
    Plugin
    Actions() []workflow.Action
}
```

Names are illustrative, not final.

### Persistence

Workflow runs should emit thread events using the existing event registry pattern.

Potential event kinds:

```text
workflow.started
workflow.step_started
workflow.step_completed
workflow.step_failed
workflow.completed
workflow.failed
action.intent_declared
action.decision_recorded
action.result_recorded
```

Do not create a separate workflow database until thread events prove insufficient.

### Runtime relationship

Workflow belongs to the broader runtime system, but not inside the low-level model/tool loop.

```text
runner.RunTurn         = one model/tool loop
runtime.Engine         = high-level turn engine over history/thread/context
workflow.Executor      = DAG/pipeline orchestration using actions and runtime turns
harness.Service        = hosts apps, sessions, workflows, channels, triggers
```

## Harness architecture

Harness should consolidate what is currently split across `terminal/cli.Load`, `app.App`, and `agent.Instance`.

Initial harness should be modest. It should not replace `app.App`; it should host it.

Responsibilities:

- load or receive app/resource composition;
- own session registry/lifecycle;
- open/resume thread-backed sessions;
- expose a channel-facing send/subscribe API;
- route work to agent/runtime/workflow execution;
- host triggers;
- install safety policy/middleware;
- emit observable events.

A first implementation can wrap current objects:

```text
harness.Service
  contains app.App
  opens agent.Instance sessions
  forwards user input to Instance.RunTurn
  forwards runner events to channel subscribers
```

Then evolve:

```text
harness.Service
  contains app composition
  owns sessions directly
  creates runtime engines via runtime.OpenThreadEngine
  hosts workflows/actions
  supports multiple channels/triggers
```

## Package evolution map

| Current package | Future role |
| --- | --- |
| `tool` | Keep as public core tool API; share concepts with actions. |
| `tools/*` | Keep generic tools; expose some as actions where useful. |
| `tools/standard` | Keep as compatibility/convenience; evolve toward `bundles/*`. |
| `toolmw` | Keep; gradually become part of broader safety architecture. |
| `runtime` | Keep turn runtime; remove concrete tool dependencies over time. |
| `runner` | Keep low-level model/tool loop. |
| `conversation` | Keep conversation projection/history model. |
| `thread` | Keep durable event/store model; add workflow events. |
| `capability` | Keep attachable stateful feature model. |
| `capabilities/planner` | Keep as built-in capability. |
| `agentcontext` | Keep context provider/render model; reuse for workflow steps. |
| `skill` | Keep instruction/reference resource model. |
| `command` | Keep slash command model; optionally expose as workflow actions. |
| `resource` | Extend contribution bundle with workflows/actions. |
| `agentdir` | Extend loader for `.agents/workflows`. |
| `app` | Keep composition root; add workflows/actions; later hosted by harness. |
| `plugins/*` | Extend plugin facets; keep first-party bundles. |
| `agent` | Keep spec and compatibility façade; migrate host/session duties outward. |
| `terminal/*` | Evolve into first channel over harness. |
| `usage` | Keep runtime usage aggregation; integrate workflow attribution later. |

## Current coupling issues to reduce

Observed top-level dependency issues:

1. `agent` imports many high-level and low-level packages: runtime, runner, terminal UI, tools/standard, thread/jsonlstore, planner, usage, skill, context providers, and llmadapter routing.
2. `runtime` imports `tools/skills` and `tools/toolmgmt`; this makes low-level runtime aware of concrete tool packages.
3. `terminal` imports `agent`, `agentdir`, `app`, `runner`, `tool`, and `usage`; it is currently both channel and composition root.
4. `app` imports `tools/standard`; this makes app composition default-heavy.

Migration strategy:

- Do not break these immediately.
- Add harness/channel/workflow boundaries alongside current code.
- Move setup paths gradually and keep compatibility shims.
- Use `go list` import checks to verify dependency direction improves.

## Verification approach

For every architectural change:

```bash
go test ./...
```

For resource/app/channel changes:

```bash
go run ./cmd/agentsdk discover [path]
go run ./cmd/agentsdk run [path]
```

For dependency direction:

```bash
go list -f '{{.ImportPath}} -> {{join .Imports " "}}' ./...
```

## Design trade-offs

### Evolution vs clean rewrite

A clean rewrite would produce prettier packages faster but would risk breaking working runtime, terminal, resource, plugin, skill, and safety behavior.

Recommendation: evolve in place. Add missing seams, then move code behind those seams.

### Harness vs app.App

`app.App` already composes agents, resources, commands, plugins, tools, and skill sources. A harness should not duplicate this.

Recommendation: harness hosts `app.App` first. Later, if `app.App` becomes too session-oriented or not session-oriented enough, split responsibilities deliberately.

### Workflow as new system vs extension of resources/plugins/thread

A standalone workflow system would be faster to prototype but would duplicate discovery, plugin, persistence, and observability concepts.

Recommendation: workflow/action should extend `resource.ContributionBundle`, `app.Plugin`, app manifests/resource roots, thread events, and existing runtime/context/tool concepts.

### Actions vs tools

Tools are model-callable; actions are workflow-callable. Combining them too early may leak model-turn assumptions into workflows. Separating them too hard may duplicate schema, result, intent, and middleware machinery.

Recommendation: introduce actions as a workflow concept but reuse tool schema/result/intent/middleware patterns aggressively.
