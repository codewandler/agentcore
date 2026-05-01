# 03 — agentsdk Roadmap

## Purpose

This roadmap turns the vision and architecture into incremental work. It is grounded in what agentsdk already has: runtime, runner, thread persistence, resource discovery, app/plugin composition, terminal execution, tools, skills, capabilities, context providers, and safety primitives.

The roadmap should therefore be read as an evolution plan, not a greenfield build plan.

## Guiding rules

1. Keep `go test ./...` green after every change.
2. Preserve current `agentsdk run` and `agentsdk discover` behavior while internals evolve.
3. Reuse existing packages before creating new parallel systems.
4. Extend `resource.ContributionBundle`, `app.Plugin`, thread events, and app manifests rather than bypassing them.
5. Add missing boundaries before moving large amounts of code.
6. Let real apps such as `cs-bot` validate abstractions before generalizing them.
7. Keep compatibility shims or migration notes for public API moves.

## Current foundation to reuse

Before adding anything, recognize the reusable pieces already present:

| Existing subsystem | Reuse for future work |
| --- | --- |
| `runtime.Engine`, `runner.RunTurn` | model/tool turn execution; workflow model-step action implementation. |
| `conversation`, `thread`, `thread/jsonlstore` | durable session and future workflow/action event persistence. |
| `runtime.ThreadRuntime` | thread-bound capability and context replay; future harness sessions. |
| `tool`, `activation`, `tools/*`, `toolmw` | tool/action schema, execution, intent, middleware, risk assessment. |
| `capability`, `capabilities/planner` | attachable stateful features; planner remains a capability, not a workflow. |
| `agentcontext` | selected context for turns and future workflow steps. |
| `skill` | instruction/reference resources; not a workflow replacement. |
| `command` | slash commands and possible command actions. |
| `agentdir`, `resource` | workflow/action resource discovery should extend this. |
| `app`, `plugins/*` | workflow/action registration should extend this plugin/app model. |
| `terminal/*` | first channel; should migrate onto harness/session APIs. |
| `agent.Spec`, `agent.Instance` | current blueprint/session façade; migrate responsibilities gradually. |

## Milestone 0 — Documentation and alignment

Status: in progress.

Deliverables:

- `docs/01_VISION.md`
- `docs/02_ARCHITECTURE.md`
- `docs/03_ROADMAP.md`

Acceptance criteria:

- The project has a top-level product direction.
- Existing subsystems and their future roles are documented.
- The runtime/workflow/harness/channel/trigger distinction is documented.
- The default workflow resource location is documented as `.agents/workflows/*.yaml`.
- Migration paths are described, not just new package ideas.

Verification:

```bash
go test ./...
```

## Milestone 1 — Promote and preserve dogfood apps

Goal: distinguish real first-party agentic apps from small examples without breaking current workflows.

Current state:

- `examples/engineer` is already a useful resource-only app.
- It uses current agentdir/app manifest/resource behavior.
- It is effectively a dogfood coding/architecture/devops agent.

Tasks:

1. Create an `apps/` directory for first-party dogfood apps.
2. Move or copy `examples/engineer` to:

   ```text
   apps/engineer/
   ```

3. Keep compatibility documentation for the old example path.
4. Add or reserve:

   ```text
   apps/builder/
   ```

5. Keep `examples/` for small instructional examples.
6. Update README, AGENTS notes, and example references.

Acceptance criteria:

- `agentsdk run apps/engineer` works.
- The engineer app remains resource-only unless/until it needs Go extensions.
- Documentation explains why engineer is a dogfood app, not just a tiny example.
- Existing examples continue to run or have clear migration notes.

Verification:

```bash
go test ./...
go run ./cmd/agentsdk discover apps/engineer
go run ./cmd/agentsdk run apps/engineer --help
```

## Milestone 2 — Extend resource discovery for workflows

Goal: introduce workflow specs through the existing resource pipeline.

Current state:

- `agentdir` loads agents, commands, and skills from `.agents`, `.claude`, and plugin roots.
- `resource.ContributionBundle` normalizes discovered contributions.
- `app.App` consumes contribution bundles.

Tasks:

1. Add workflow resource representation to `resource.ContributionBundle`.
2. Add workflow metadata/source provenance similar to skills/commands.
3. Extend `agentdir` to discover:

   ```text
   .agents/workflows/*.yaml
   ```

4. Keep workflow loading declarative-only at first; do not require execution.
5. Update `agentsdk discover` to show discovered workflow resources.
6. Update `docs/RESOURCES.md` with the chosen workflow resource convention and note whether it is agentsdk-specific.

Acceptance criteria:

- A workflow YAML file under `.agents/workflows/` is discoverable.
- Discovery output includes workflow name/source/diagnostics.
- Existing agent/command/skill discovery is unchanged.

Verification:

```bash
go test ./agentdir/... ./resource/... ./cmd/agentsdk/...
go run ./cmd/agentsdk discover testdata-or-example-path
```

## Milestone 3 — Add workflow/action core model

Goal: define workflows and actions as first-class domain concepts while reusing tool patterns.

Current state to reuse:

- `tool.TypedTool` already has typed params, schema generation, execution, result formatting, and intent declaration patterns.
- `command.Command` already models named app actions with policy and result kinds.
- `thread.EventDefinition` already supports registering typed event payloads.

Tasks:

1. Add `workflow` package.
2. Define domain types:

   ```text
   Workflow
   Pipeline
   Step
   Edge
   ActionRef
   Action
   ActionKind
   Input schema
   Output schema
   ```

3. Decide how much of `tool.Result` can be reused for `ActionResult`.
4. Decide how `tool.Intent` generalizes to action intent without duplicating risk machinery.
5. Support Go-defined workflows and actions first.
6. Add tests for model validation, step references, and simple pipeline construction.

Acceptance criteria:

- Workflows/actions can be constructed in Go.
- A pipeline is represented as a workflow with sequenced edges.
- Invalid workflow references are caught by validation.
- Action schema/intent design reuses existing tool concepts where possible.

Verification:

```bash
go test ./workflow/...
go test ./tool/... ./command/...
```

## Milestone 4 — Extend app/plugin composition for workflows/actions

Goal: make workflows/actions part of the existing app/plugin model.

Current state:

- `app.Plugin` facets already contribute commands, agent specs, tools, skill sources, context providers, and middleware.
- `app.App` owns command registry, agent specs, tool catalog, skill sources, context providers, plugin registrations, and resource bundles.

Tasks:

1. Add plugin facets for workflows/actions.
2. Add app-level registries for workflows/actions.
3. Register workflow/action contributions from resource bundles.
4. Register workflow/action contributions from plugins.
5. Add app APIs to list/get workflows/actions.
6. Keep existing tool/command/skill behavior unchanged.

Acceptance criteria:

- A plugin can contribute an action implementation.
- A resource bundle can contribute a workflow definition.
- `app.App` can resolve workflow definitions and action implementations.
- No separate workflow plugin system exists.

Verification:

```bash
go test ./app/... ./plugins/... ./resource/...
```

## Milestone 5 — Minimal workflow executor

Goal: execute a simple sequential pipeline using existing runtime/tool infrastructure.

Current state to reuse:

- `runtime.Engine` can run model/tool turns.
- `tool.Tool` can execute typed operations.
- `command.Command` can run slash-command-like app actions.
- `agentcontext.Manager` can provide context fragments.
- `thread.Event` can persist execution records.

Tasks:

1. Implement a minimal workflow executor for sequential pipelines.
2. Support initial action kinds:

   - model/agent turn action using `runtime.Engine` or `agent.Instance` initially;
   - tool action wrapping `tool.Tool`;
   - command action wrapping `command.Command` where appropriate;
   - no-op/transform action for tests.

3. Emit workflow/action events to the existing thread event log when a thread is available.
4. Add per-step input/output passing.
5. Add basic output validation.
6. Defer parallel DAG execution until sequential pipeline semantics are proven.

Acceptance criteria:

- A Go-defined pipeline can execute end-to-end.
- Output from one step can feed the next.
- Tool/action intent can be inspected before execution.
- Execution is observable through events.
- Thread-backed runs can persist workflow events.

Verification:

```bash
go test ./workflow/... ./runtime/... ./thread/...
```

## Milestone 6 — Minimal harness wrapping existing app/agent flow

Goal: consolidate current app/session setup without rewriting runtime.

Current state:

- `terminal/cli.Load` resolves resources and constructs `app.App` and `agent.Instance`.
- `app.App` registers resources/plugins and instantiates agents.
- `agent.Instance` owns runtime/session setup.

Tasks:

1. Add `harness` package.
2. First implementation should wrap existing objects:

   ```text
   harness.Service
     contains app.App
     opens/owns agent.Instance sessions
     forwards input to Instance.RunTurn
     emits runner events
   ```

3. Move the reusable parts of `terminal/cli.Load` toward harness loading functions.
4. Keep `terminal/cli.Load` as compatibility wrapper initially.
5. Add session IDs and thread/session store handling through harness APIs where possible.

Acceptance criteria:

- Harness can load resources using existing `agentdir`/`resource`/`app` paths.
- Harness can instantiate the default agent through existing `app.App` APIs.
- Harness can run a turn and expose events.
- `agentsdk run` behavior remains unchanged.

Verification:

```bash
go test ./harness/... ./terminal/... ./app/... ./agent/...
go run ./cmd/agentsdk run apps-or-examples-path
```

## Milestone 7 — Terminal becomes first channel over harness

Goal: make the existing terminal stack the first implementation of a channel boundary.

Current state:

- Terminal code works but directly performs resource/app/agent setup.
- Runner events already map well to terminal rendering.

Tasks:

1. Add minimal `channel` package.
2. Define channel host/session interfaces based on what terminal actually needs.
3. Adapt terminal REPL/UI to use harness APIs.
4. Keep terminal rendering in `terminal/ui`.
5. Keep CLI flags and UX stable.

Acceptance criteria:

- Terminal does not need to know low-level runtime construction details.
- Terminal still supports tasks, REPL, session resume, verbose/debug output, and slash commands.
- Harness can theoretically host another channel with the same session API.

Verification:

```bash
go test ./channel/... ./terminal/... ./harness/...
go run ./cmd/agentsdk run apps/engineer
```

## Milestone 8 — Safety policy expansion

Goal: evolve existing tool intent/middleware/cmdrisk into a broader safety layer.

Current state:

- `tool.Intent` exists.
- `IntentProvider` exists.
- `toolmw.CmdRiskAssessor` exists.
- Bash declares intent with cmdrisk analysis.
- Standard toolset can configure risk analyzer.
- Terminal currently has log-only risk middleware.

Tasks:

1. Define safety decision types: allow, deny, require approval, require sandbox, require network policy.
2. Extend intent concepts to workflow actions.
3. Add approval interfaces that terminal channel can satisfy first.
4. Add audit event payloads using thread events.
5. Keep current tool middleware working.
6. Prepare adapter interfaces for Bubblewrap/network interception, but do not implement all sandboxing yet.

Acceptance criteria:

- Tool and action calls can be assessed before execution.
- Terminal can display/handle approval prompts for high-risk actions.
- Decisions are observable and, when thread-backed, persisted.
- Existing cmdrisk behavior still works.

Verification:

```bash
go test ./tool/... ./toolmw/... ./tools/shell/... ./workflow/...
```

## Milestone 9 — Trigger interface and interval trigger

Goal: support background/event-driven work while reusing harness sessions.

Current state:

- No generic trigger abstraction yet.
- Harness/session APIs from prior milestones should provide a target.

Tasks:

1. Add `trigger` package.
2. Define minimal trigger/sink interfaces.
3. Implement interval trigger as first proof.
4. Route trigger events to harness sessions or workflows.
5. Persist/observe trigger-caused runs with source metadata.

Acceptance criteria:

- A trigger can start/resume work through harness.
- Trigger source metadata appears in thread/runtime events.
- Trigger implementation is separate from channels.

Verification:

```bash
go test ./trigger/... ./harness/... ./workflow/...
```

## Milestone 10 — cs-bot MVP validation

Goal: validate the architecture with a real support/ticket triage application in `~/babelforce/projects/cs-bot`.

Current cs-bot should not wait for every agentsdk refactor. It should use existing agentsdk pieces and feed lessons back.

Recommended scope:

1. Pick one connector first: Jira or Zendesk.
2. Implement connector operations as tools and/or actions.
3. Create a triage agent spec using existing resource loading.
4. Create a ticket triage workflow as Go-defined workflow first or `.agents/workflows/ticket_triage.yaml` if YAML support is ready.
5. Run triage manually from CLI first.
6. Add approval before any write/update operation.
7. Add interval/cron-like trigger only after manual flow works.
8. Persist session/thread state.

Suggested first workflow:

```text
load ticket
  -> summarize/classify
  -> determine severity/team/action
  -> propose update
  -> approval gate for writes
  -> apply label/comment/assignment
  -> emit result
```

Acceptance criteria:

- A real ticket can be loaded.
- The workflow produces structured triage output.
- Writes require approval.
- The run is observable and persisted.
- Any missing generic abstractions are identified before extracting them into agentsdk.

Verification:

```bash
# in cs-bot
go test ./...
```

## Milestone 11 — Builder app MVP

Goal: ship the first useful `agentsdk build` using existing app/resource/runtime pieces.

Current state to reuse:

- `apps/engineer`/dogfood agent pattern.
- Agentdir/app manifest/resource formats.
- Filesystem/shell/git tools.
- Planner capability.
- Terminal channel.
- Future workflow/action scaffolding.

Tasks:

1. Create `apps/builder` as an agentsdk app.
2. Add `agentsdk build` command path.
3. Start with guided scaffolding, not magic generation.
4. Generate resource-only apps first.
5. Add hybrid Go plugin/action/tool scaffolding next.
6. Add deployment templates later.
7. Use `agentsdk discover` and test commands as verification steps.

Builder output levels:

1. Resource-only YAML/Markdown.
2. Hybrid resources plus generated Go extension code.
3. Full app with custom Go harness code.
4. Deployment-ready app with Docker/Helm/CI assets.

Acceptance criteria for MVP:

- User can run `agentsdk build`.
- Builder asks basic requirements questions.
- Builder creates an agentdir/app manifest.
- Builder can create a workflow skeleton.
- Builder can run or print verification steps.

Verification:

```bash
go test ./...
go run ./cmd/agentsdk build
```

## Milestone 12 — HTTP/SSE channel

Goal: prove that terminal is not special by adding a second channel.

Prerequisite:

- Harness/session API is stable enough from terminal migration.

Tasks:

1. Add HTTP request/response channel.
2. Add SSE event stream for runtime/workflow events.
3. Keep protocol minimal and versioned.
4. Avoid full web UI until channel semantics are stable.

Acceptance criteria:

- A client can send a request to an agent session over HTTP.
- A client can receive streamed events over SSE.
- Terminal and HTTP channels share harness/session behavior.

Verification:

```bash
go test ./channels/http/... ./harness/...
```

## Milestone 13 — Dependency cleanup

Goal: reduce coupling after new boundaries have proven useful.

Tasks:

1. Remove concrete tool imports from `runtime`.
2. Move terminal-specific behavior out of `agent`.
3. Shrink `agent.Instance` toward a compatibility façade over harness/session/runtime pieces.
4. Move default-heavy app wiring out of `app.New` where appropriate.
5. Split broad standard bundles into `bundles/` while keeping compatibility exports.
6. Move product/environment integrations into `adapters/` as they are added.

Acceptance criteria:

- `runtime` no longer imports concrete `tools/*` packages.
- `agent` no longer imports terminal UI.
- Terminal is a channel over harness.
- Existing public API either still works or has explicit migration notes.

Verification:

```bash
go list -f '{{.ImportPath}} -> {{join .Imports " "}}' ./...
go test ./...
```

## Deferred work

- Full web UI.
- Advanced TUI.
- gRPC/WebSocket/telnet channels.
- Parallel DAG workflow executor.
- Durable workflow pause/resume/wait semantics.
- Advanced sub-agent orchestration semantics.
- Bubblewrap sandbox adapter.
- Network interception adapter.
- Remote plugin distribution/trust model.
- Hosted control plane, if ever desired.

## Near-term recommendation

The next practical sequence should be:

1. finalize these docs;
2. promote/preserve engineer as dogfood app;
3. extend resource discovery for workflows;
4. add workflow/action core model;
5. extend app/plugin composition;
6. add minimal workflow executor;
7. introduce harness as wrapper over existing app/agent flow;
8. migrate terminal onto harness;
9. validate with cs-bot;
10. only then perform larger dependency cleanup.

This keeps the architecture grounded in working code while moving toward the product vision.
