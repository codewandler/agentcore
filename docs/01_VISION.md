# 01 — agentsdk Vision

## Purpose

agentsdk is a Go framework, runtime, and builder for secure, reliable agentic applications.

It is already more than a tool library: the repository contains a turn runtime, durable thread/session state, resource discovery, app manifests, plugins, skills, commands, terminal execution, standard tools, safety primitives, context providers, and a dogfood engineer agent. The future design should evolve these existing pieces into a clearer product architecture rather than replace them with unrelated new abstractions.

agentsdk should support three related product roles:

1. **Agent runtime** — the reusable execution substrate for model turns, tools, context, state, safety, capabilities, events, and persistence.
2. **Agent development kit** — the APIs, resource formats, plugins, examples, and app composition model used by developers to build agentic apps in Go and with declarative resources.
3. **Agent builder** — an agent-powered product surface (`agentsdk build`) that interviews users about a use case and creates agents, workflows, actions, plugins, repositories, deployment assets, and operational configuration.

The long-term product promise:

> A user describes an agentic use case — for example, ticket triage in Jira/Zendesk — and agentsdk helps turn that use case into a working, safe, observable, deployable agentic application.

agentsdk should provide the runtime, safety model, workflow/action model, app packaging, resource discovery, and builder experience so each application does not reinvent the same infrastructure.

## Product north star

agentsdk should make agentic applications boring to build and safe to operate.

A good agentsdk application should have:

- declarative agent/app configuration where useful;
- Go-code construction paths for embedded applications;
- typed tools, actions, inputs, and outputs;
- explicit side-effect policy and approval gates;
- durable thread/session state;
- reproducible context rendering;
- observable runtime and workflow events;
- reliable workflows with schemas and constrained execution;
- multiple channels for human/system interaction;
- triggers for scheduled or event-driven background work;
- packaging/deployment paths suitable for real services.

## What exists today

The current repository already provides the foundation for the vision:

- `runtime` and `runner` execute model/tool turns over `llmadapter/unified.Client`.
- `conversation` models branchable history, projected items, compaction, and provider continuations.
- `thread` provides append-only thread events, branches, stores, memory store, and JSONL persistence.
- `runtime.ThreadRuntime` binds live threads to capability state and context render replay.
- `tool` defines model-callable tools, JSON schemas, results, intent declaration, middleware hooks, and unified conversion.
- `tools/*` provides filesystem, shell, git, web, vision, phone, JSON query, todo, turn, skill, and tool management tools.
- `toolmw` and `cmdrisk` integration provide an early risk/safety layer.
- `capability` and `capabilities/planner` provide attachable, event-sourced capability state.
- `agentcontext` and `agentcontext/contextproviders` provide context managers, render records, diffs, and built-in context providers.
- `skill` supports Agent Skills-compatible skill directories, references, repositories, and activation events.
- `command` supports slash commands and a `command_run` tool bridge for agent-callable commands.
- `agentdir` and `resource` load `.agents`, `.claude`, app manifests, local/global resources, declarative git sources, and normalized contribution bundles.
- `app` composes resource bundles, commands, plugins, tools, skills, context providers, middleware, and agent specs into running app instances.
- `plugins/*` already define first-party plugin bundles for git, skills, tool management, vision, and standard plugin sets.
- `terminal/cli`, `terminal/repl`, and `terminal/ui` provide the current terminal channel and `agentsdk run` experience.
- `examples/engineer` is a practical dogfood resource bundle used as a coding/architecture/devops agent.

The product vision is therefore evolutionary: clarify boundaries, reuse these foundations, and add missing concepts only where the current model cannot express the future product.

## What agentsdk is

agentsdk is both a library and an executable product surface.

As a **library**, it lets Go developers build agents, tools, actions, workflows, plugins, channels, triggers, and app harnesses.

As a **runtime**, it runs agent turns, executes tools/actions, renders context, persists state, enforces safety policies, and emits events.

As a **resource/app format**, it can load agent directories, app manifests, skills, commands, workflow specs, and plugin-provided contributions.

As a **builder**, it should eventually generate full agentic apps from requirements: resource-only apps, mixed YAML/Go apps, custom Go code, tests, and deployment artifacts.

## What agentsdk is not

agentsdk should not become a monolithic SaaS product or a pile of product-specific integrations inside the core runtime.

The core should stay reusable. Jira, Zendesk, Slack, email, browser automation, hosted web UIs, deployment targets, and cloud-specific integrations should be modeled as adapters, plugins, bundled apps, or generated application code unless they are genuinely universal primitives.

## Product surfaces

### `agentsdk run`

Runs an agentic app from Go code or from resources such as agent directories and app manifests.

This exists today through `cmd/agentsdk`, `terminal/cli`, `app`, `agentdir`, `resource`, and `agent.Instance`. It should evolve without breaking current behavior. Internally, more of the shared app/session/channel lifecycle should move behind a harness boundary so terminal, HTTP/SSE, TUI, WebSocket, gRPC, telnet, and proprietary channels can reuse the same host logic.

### `agentsdk discover`

Inspects discovered resources without running an agent.

This already exists and should remain a key debugging/product surface for app manifests, resource roots, external sources, skills, commands, agents, and future workflow/action resources.

### `agentsdk build`

An agent-powered builder that helps users create agentic apps.

The builder should be able to:

- ask requirements questions;
- identify required agents, tools, actions, connectors, workflows, channels, and triggers;
- create agent specs and app manifests;
- create workflow YAML specs where declarative resources are enough;
- scaffold Go plugins/actions/tools/connectors where code is needed;
- generate tests;
- create Docker/build/deployment assets from templates;
- initialize or update a git repository;
- run verification commands and report gaps.

Builder output should support multiple complexity levels:

1. **Resource-only** — YAML/Markdown/spec files only.
2. **Hybrid** — declarative resources plus generated Go plugins/actions/tools.
3. **Full app** — custom Go app/harness code, tests, packaging, and deployment assets.
4. **Deployment-ready** — Docker, Helm, CI/CD, service manifests, and environment-specific configuration generated from templates.

The builder itself should be an agentsdk app and dogfood the runtime, workflows, safety, tools, and packaging model.

### Harness / daemon

The harness is the host for complete agentic applications.

Today, this role is partially split across `terminal/cli.Load`, `app.App`, and `agent.Instance`. Those existing pieces should evolve into a clearer harness/session abstraction rather than being discarded.

The harness should initialize configured agents, channels, stores, datasources, triggers, tools, actions, policies, and workflows. It owns process-level lifecycle and exposes a control plane to channels and external systems.

The harness may run embedded in a CLI process or as a daemon/service. The public concept should be **harness**. **Daemon** is one deployment mode.

## Core product concepts

### Agent

An agent is a configured actor with instructions, tools/actions, skills, capabilities, model policy, context sources, and persistent session state.

Today this is represented by `agent.Spec` for the declarative blueprint and `agent.Instance` for a running session-backed object. The future architecture should preserve that distinction while reducing the amount of terminal/app wiring inside `agent.Instance`.

### App

An app is a composition of agents, commands, plugins, tools, skill sources, context providers, middleware, and resource bundles.

Today this is `app.App`. It is already a user-facing composition root. The future harness should build on this role, not bypass it. Over time, process/session/channel lifecycle may move from terminal/agent packages into harness while `app.App` remains the composition model.

### Resource bundle

A resource bundle is a normalized set of discovered contributions: agent specs, commands, skill sources, tool contributions, hooks, permissions, diagnostics, and future workflow/action contributions.

Today this is `resource.ContributionBundle`, produced by `agentdir` and consumed by `app.App`. Workflow/action resources should extend this existing discovery model.

### Plugin

A plugin is a Go-code contribution bundle.

Today `app.Plugin` and its facets contribute commands, agent specs, tools, skill sources, context providers, agent-scoped context providers, global tool middleware, and targeted tool middleware. Future workflow/action contributions should become additional plugin facets rather than a parallel plugin system.

### Tool

A tool is a model-callable function. Tools are how the model performs side effects or queries external state during a turn.

Tools should declare intent before execution so safety policy can assess, approve, constrain, or reject the call.

### Action

An action is a named atomic operation with typed input and output. Actions are workflow primitives.

Actions are implemented in Go, have a kind/type, and can be referenced by workflows and pipelines. Examples of action kinds may include command execution, HTTP request, model step, tool call, transform, approval, or domain-specific operations.

Tools and actions are related but not identical:

- a **tool** is exposed to the model as a callable function during a turn;
- an **action** is invoked by workflow orchestration as a typed step;
- one implementation may be exposed as both a tool and an action when appropriate.

### Workflow

A workflow is a reliable, typed, inspectable execution graph for agentic applications.

It exists for cases where a free-form prompt, command, or skill is too ambiguous. Workflows compose actions into simple pipelines or more complex DAGs.

A pipeline is just a sequenced DAG.

Workflow definitions may live wherever resource discovery can find them. The default filesystem convention for YAML specs should be:

```text
.agents/workflows/*.yaml
```

An embedded Go application may construct workflows directly in code instead of using YAML. Plugins and resource discovery should make both paths first-class.

### Capability

A capability is an attachable feature module that can provide tools, context, and event-sourced state. Capabilities are useful for stateful agent features such as planning.

Today the planner capability is a concrete example. Workflows should not replace capabilities; they address a different concern. Capabilities extend an agent/session, while workflows orchestrate multi-step execution.

### Skill

A skill is an instruction/reference resource. Skills guide behavior and provide context. They are not the same as tools or workflows.

Today skills and exact `references/` paths can be activated at runtime and persisted across resumed sessions.

### Command

A command is a user/app slash-command action. Commands may optionally be exposed to agents through a tool bridge, but their primary role is user/app interaction.

Today command files are resources and `command.Tool` exposes explicitly agent-callable commands as `command_run`.

### Channel

A channel exposes agents to users or external systems.

The current terminal CLI/REPL/UI stack is the first channel, although it is not yet factored as a generic channel. Future channels should reuse the same harness/session/control API.

Examples:

- terminal/REPL;
- TUI;
- web UI;
- HTTP request/response;
- SSE event streams;
- WebSocket;
- gRPC;
- telnet;
- proprietary RPC;
- chat surfaces.

A channel is not a tool. A channel is ingress/egress for humans or systems.

### Trigger

A trigger starts or resumes agent work from events rather than direct user input.

Examples:

- cron/scheduler;
- fixed interval loops;
- webhooks;
- file watchers;
- queue events;
- email events;
- Slack events;
- system monitor ticks.

A trigger is not a channel. A channel is an interaction surface; a trigger is an event source.

### Adapter

An adapter connects agentsdk to a third-party service or environment-specific system.

Examples:

- Jira/Zendesk/Slack/email connectors;
- Tavily search;
- SIP/phone;
- Bubblewrap sandboxing;
- network interception;
- cloud/deployment providers.

## Safety as a differentiator

Safety should be a first-class product property, not a per-tool afterthought.

agentsdk already has a foundation: tool intent, middleware hooks, cmdrisk assessment, shell intent declaration, and standard toolset risk analyzer configuration. The future safety layer should generalize this across tools, actions, workflows, channels, and harness policies.

agentsdk should support:

- tool/action intent declaration;
- risk assessment;
- command risk analysis;
- approval gates;
- policy decisions;
- sandbox execution;
- filesystem boundaries;
- network policy/interception;
- secret handling/redaction;
- audit logs.

The intended flow:

```text
tool/action request
  -> declare intent
  -> assess risk
  -> apply policy
  -> optionally ask approval
  -> optionally constrain with sandbox/network policy
  -> execute
  -> persist/audit result
```

## Dogfood applications

The engineer agent is too important to remain only a tiny example. It is a dogfood app: the agent used to build agentsdk itself.

Recommended direction:

```text
apps/engineer/   first-party dogfood coding/architecture/devops agent
apps/builder/    first-party builder agent used by `agentsdk build`
examples/        small instructional examples only
```

Examples should remain small and educational. Dogfood apps can be larger and product-like.

## Design principle

Keep the layering simple and evolutionary:

```text
Existing runtime executes turns.
Existing thread/conversation packages persist state.
Existing app/resource/plugin packages compose applications.
Existing terminal code is the first channel.
Existing tool/intent/middleware code is the safety seed.
Workflow adds reliable action orchestration.
Harness consolidates host/session/channel/trigger lifecycle.
Builder creates apps from these pieces.
```
