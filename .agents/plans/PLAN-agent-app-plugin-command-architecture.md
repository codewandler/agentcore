# PLAN: Agent, App, Plugin, and Command Architecture

Status: in progress  
Created: 2026-04-26

## Goal

Reshape agentsdk so it can build slim agent applications like miniagent from
reusable SDK components, while also supporting filesystem-described agent
bundles and plugin-style contributions.

This iteration should prefer clean architecture and open ecosystem compatibility
over migration shortcuts. Do not add new adapters, fallback compatibility
layers, or patch-over wrappers just to preserve temporary shapes. If an existing
prototype API is wrong for the new design, replace it with the intended API and
migrate callers deliberately.

The target architecture separates execution, agent identity, app composition,
commands, plugins, and terminal frontends:

```text
runtime.Engine       low-level model/tool execution engine
agent.Spec           declarative agent identity/config
agent.Instance       running session-backed agent
app.App              composition root: plugins, commands, agents, sessions
command.Registry     command primitives and dispatch
skill.Repository     discovered and loaded skill set for one agent
tool.Catalog         app-owned catalog of available tools by name
terminal/repl        terminal frontend over app.App
terminal/ui          terminal rendering
```

miniagent should eventually be an app/distribution with one primary coding
agent, not a library package that owns generic agent mechanics. Its resources
should be usable in two equivalent ways:

```sh
agentsdk run ../miniagent/agent
miniagent
```

The first path uses a generic built-in SDK app that loads resources from disk.
The second path is miniagent's branded `main.go`, but it should load the same
resources and only add product packaging such as CLI defaults, session path
policy, binary name, and optional Go-defined extensions. If miniagent does not
provide Go-defined tools/configuration, both launch paths should have identical
agent functionality.

## Core Terminology

- **Engine**: low-level runtime that executes model/tool turns against a
  conversation session.
- **Agent spec**: declarative identity/config for one agent: system prompt,
  model defaults, tools, skill sources/default skills, and optionally command
  contributions.
- **Agent instance**: a running agent created from a spec, with session,
  history, usage, runtime engine, and its own resolved skill repository/list.
- **Resource source**: a typed filesystem or embedded root that can contribute
  agents, commands, skills, tools, or plugin metadata. Sources are standard-aware
  and can represent Claude Code layout, Agent Skills layout, plugin layout, or
  explicit SDK-provided embedded resources.
- **Skill source**: a resource source that contributes Agent Skills-compatible
  `SKILL.md` directories to an agent.
- **Skill repository**: the resolved skill catalog and loaded skill set owned by
  one agent instance.
- **Plugin/bundle**: installable contribution package. It can provide commands,
  skills, tools, and agent specs.
- **App**: user-facing composition root. It loads plugins/bundles, owns command
  registries, creates agent instances, applies session policy, and exposes a
  frontend/API.
- **Tool catalog**: app-owned set of standard and contributed tools. Agent specs
  select from this catalog by name/pattern; agent instances receive the resolved
  toolset.
- **Frontend**: terminal REPL, TUI, HTTP, Slack, or another interface over the
  app. A frontend presents input/output; it does not own commands, sessions, or
  agent selection.
- **Runner command**: a generic SDK CLI entry point such as `agentsdk run` that
  resolves app/plugin/resource sources from one or more paths, constructs an app,
  and runs a selected/default agent.
- **Compatibility source**: a loader for an existing external convention, such
  as Claude Code's `.claude/` project/user directories. Compatibility sources
  are first-class typed sources, not fallback parsing hacks.

## Execution Plan

### Phase 1: Command and Terminal Foundations

- [x] Add `command` package with `Spec`, `Command`, typed `Result`, `Registry`,
  and slash command parsing.
- [x] Add `command/markdown` package for Markdown-backed prompt commands with
  YAML frontmatter and template rendering.
- [x] Add `terminal/ui` package for step display, runner event rendering, usage
  formatting, and terminal error/session output.
- [x] Add `terminal/repl` package with generic command dispatch and built-in
  commands.
- [x] Add focused tests for parser, registry, Markdown commands, REPL dispatch,
  and high-level agent behavior.
- [x] Run `env GOCACHE=/tmp/go-cache go test ./...` in agentsdk.

### Phase 2: Rename Runtime Execution Primitive

- [x] Rename `runtime.Agent` implementation to `runtime.Engine`.
- [x] Remove the temporary `runtime.Agent = Engine` alias before migrating
  miniagent. New code must use `runtime.Engine` directly.
- [x] Rename receiver variables and docs in `runtime` from agent-oriented terms
  to engine-oriented terms where that improves clarity.
- [x] Update internal agentsdk callers to prefer `runtime.Engine`.
- [x] Keep public constructors compatible:
  - `runtime.New(...) (*Engine, error)`
  - `runtime.Must(...) *Engine`
- [x] Verify external consumers compile against `runtime.Engine` after their
  migration, not through an alias.
- [x] Current status: alias removed.

### Phase 3: Split Agent Concerns

- [x] Convert the current prototype `agent.Agent` package into clearer types:
  - `agent.Spec`
  - `agent.Instance`
  - `agent.Option`
- [x] Remove the compatibility command registry from `agent.Instance`.
- [x] Keep `agent.Instance` focused on:
  - resolved model/inference config
  - system prompt/spec materialization
  - runtime engine construction
  - conversation session attachment
  - usage tracking
  - tool context and active toolset
- [x] Keep app/frontends responsible for user input routing and command
  execution.
- [x] Remove the temporary `agent.Agent = Instance` alias before migrating
  miniagent. New code must use `agent.Instance` for running agents and
  `agent.Spec` for declarations.
- [x] Current status: alias removed.

### Phase 4: Add App Composition Root

- [x] Add `app` package with `App` as the user-facing container.
- [x] App owns:
  - command registry
  - loaded plugins/bundles
  - agent specs
  - agent instances
  - selected/default agent
  - input routing
- [x] Add explicit app-level session policy before miniagent migration. Branded
  runners should configure session directory, resume behavior, and cache key
  prefix through app construction instead of reimplementing agent/session setup.
- [x] Make app the single owner of turn sequencing for both programmatic calls
  and terminal REPL calls. Frontends should not maintain independent turn IDs.
- [x] Define app input routing:
  ```text
  slash input      -> command registry
  plain input      -> default/selected agent
  command result   -> text, reset, exit, agent turn, or app action
  ```
- [x] Move built-in commands from `terminal/repl` to app-level defaults:
  - `/help`
  - `/new`
  - `/session`
  - `/quit`
  - `/turn`
- [x] Remove terminal/repl fallback built-in registration. The REPL should not
  own or synthesize app commands.
- [x] Rename or reshape the `terminal/repl` target interface so it is clearly an
  app/frontend target, not an `Agent` interface with command ownership.
- [x] Have `terminal/repl` send raw user input to app-level routing instead of
  resolving commands itself. The app should return typed results/events for the
  frontend to render.
- [x] Keep terminal-specific behavior, prompts, and rendering in
  `terminal/repl` and `terminal/ui`.

### Phase 5: Command Scope and Agent-Callable Commands

- [x] Extend `command.Spec` with call policy metadata, for example:
  ```go
  type CommandPolicy struct {
      UserCallable  bool
      AgentCallable bool
      Internal      bool
  }
  ```
- [x] Ensure user-visible help only shows user-callable commands.
- [x] Ensure app slash input executes only user-callable commands. Agent-only
  commands are reserved for the app-provided command tool.
- [x] Allow app/plugin/agent specs to contribute commands, but register them in
  the app registry.
- [ ] Define agent-specific command visibility/scoping. A one-agent app can use
  global commands, but multi-agent apps need a clear rule for which slash
  commands are visible/active for the selected agent.
- [ ] Refine command registration into explicit scopes:
  - app/global user commands
  - agent-selected user commands
  - agent-callable commands exposed as tools
  - internal commands
- [x] Implement scoped command views at the app layer. `command.Registry` remains
  the primitive store; `app` creates the user-visible and agent-callable command
  views for the selected agent.
- [ ] Define command resolution order and collision behavior across scopes.
  Default collisions should be explicit errors unless a manifest/config later
  declares an override.
- [x] Expose selected agent-callable commands as tools only when the app
  explicitly enables them.
- [x] Ensure `command_run` receives only the selected agent's agent-callable
  command view, not the entire app registry.
- [x] Decide between:
  - one generic `command_run` tool with policy checks
  - generated per-command tools
- [x] Keep app/control commands such as `/quit` and `/new` non-agent-callable by
  default.
- [x] Treat built-in app commands as protected commands that resource bundles
  cannot override accidentally.

### Phase 6: Filesystem Agent Bundles and Standard-Aware Sources

- [x] Add `agentdir` loader package for agent directory structures.
- [x] Load from `fs.FS` first, not only OS paths, so Go apps can embed bundles.
- [x] Reframe `agentdir` around source types instead of one proprietary
  `.agents/` tree. Required source types:
  - Claude project source: `.claude/`
  - Claude user source: `~/.claude/`
  - agentsdk compatibility source: `.agents/`
  - plugin root source: `commands/`, `agents/`, `skills/`, `.claude-plugin/`
  - explicit embedded source for Go apps
- [x] Treat `.claude/` as the primary on-disk compatibility layout for project
  and user agents/commands. Do not make `.agents/agents` or
  `.agents/commands` the default project format.
- [x] Support Claude Code-compatible project layout:
  ```text
  .claude/
    agents/
      main.md
      reviewer.md
    commands/
      review.md
      commit.md
    skills/
      coder/
        SKILL.md
  ```
- [x] Support plugin root layout compatible with Claude Code plugins:
  ```text
  plugin/
    .claude-plugin/
      plugin.json
    agents/
    commands/
    skills/
  ```
- [x] Keep `.agents/agents` and `.agents/commands` out of the default lookup
  path for ambient project/user discovery. They may still be loaded when an
  explicitly pointed-to directory is resolved as an agentsdk compatibility
  plugin source.
- [x] Define agent spec frontmatter for fields such as:
  - name
  - description
  - model
  - max steps
  - tools
  - default skills
  - command visibility/contributions
- [ ] Validate duplicate names and alias collisions. Default behavior should be
  explicit errors, not silent first-wins. Priority/override rules can be added
  later only with explicit manifest/config support.
- [x] Change `agentdir.Bundle` to carry typed contribution values:
  - `AgentSpecs []agent.Spec`
  - `Commands []command.Command`
  - `SkillSources []skill.Source`
- [x] Add source tests that load the same fixture through OS directories and
  embedded `fs.FS`.

### Phase 6.1: Directory Resolution for `agentsdk run`

- [x] Add a resolver for `agentsdk run <path>` that classifies each input path
  in this order:
  - app source if it contains an app manifest
  - embedded plugin source from `<path>/.claude`
  - embedded plugin source from `<path>/.agents`
  - plugin root source from `<path>`
- [x] Define app manifest lookup names before implementing. Initial supported
  names:
  ```text
  app.manifest.json
  agentsdk.app.json
  ```
- [x] If an app manifest exists, load only what the manifest declares. The
  manifest owns plugin/resource ordering and can reference local directories
  first; git/http plugin references remain planned but not required for this
  iteration.
- [x] If no app manifest exists, probe `<path>/.claude` and `<path>/.agents` as
  plugin roots. Load both when both exist, in deterministic order:
  - `.claude`
  - `.agents`
- [x] Distinguish explicit run-target probing from ambient discovery. Explicit
  `agentsdk run <path>` may load `<path>/.agents/{agents,commands,skills}`;
  ambient project/user discovery only treats `.agents/skills` as a default
  compatibility source.
- [x] If neither embedded plugin root exists, treat `<path>` itself as a plugin
  root and look for root-level `agents/`, `commands/`, `skills/`, and
  `.claude-plugin/plugin.json`.
- [x] Missing resource subdirectories inside a plugin root are non-fatal.
- [x] Duplicate agents, commands, command aliases, and skills across resolved
  sources are explicit errors unless an app manifest later declares override
  policy.
- [x] Define default agent resolution in deterministic order:
  - explicit CLI `--agent <name>`
  - app manifest `default_agent`
  - exactly one discovered agent
  - conventional names, in order: `main`, `default`
  - explicit ambiguity error listing discovered agents
- [x] Add tests covering:
  - manifest wins over probing
  - `<path>/.claude` plugin source
  - `<path>/.agents` plugin source
  - both `.claude` and `.agents` load in deterministic order
  - fallback to `<path>` as plugin root
  - missing resource subdirectories are ignored
  - duplicate resource names fail clearly
  - default agent resolution order
  - ambiguous multi-agent directories require `--agent`

### Phase 7: Plugin/Bundle Contributions

- [x] Define plugin/bundle interface for code-defined contributions:
  ```go
  type Plugin interface { Name() string }
  ```
- [x] Add optional contribution interfaces:
  - commands
  - skills
  - tools
  - agent specs
- [x] Add an app-owned tool catalog. It should include SDK standard tools and
  plugin-contributed tools.
- [x] Wire `ToolsPlugin` into the app tool catalog. Agent specs select enabled
  tools from the catalog by name/pattern before app constructs the
  `agent.Instance`.
- [ ] Add app hook contribution interface.
- [x] Support data-only plugin directories following the generalized
  plugin root layout.
- [x] Support root-level plugin resource directories:
  ```text
  plugin/
    commands/
    skills/
    agents/
  ```
- [x] Support Claude Code plugin manifest metadata:
  ```text
  plugin/
    .claude-plugin/plugin.json
  ```
- [ ] Add namespacing rules for plugin-contributed commands, skills, and agents.

### Phase 8: Skill Sources and Agent Skill Repositories

- [x] Revisit and simplify the existing `skill` package interfaces before
  implementation. Keep only the concepts needed by the new architecture:
  source, discovered skill, repository, loaded skill content, and explicit load
  state.
- [x] Avoid adapter/fallback layers around the existing skill prototype. If the
  current interfaces do not fit, replace them cleanly and update callers/tests.
- [x] Define the concrete SDK skill source model. It must support at least:
  - OS directory sources
  - embedded `fs.FS` sources for Go apps that embed agent resources
  - bundle/plugin-provided sources
  - app default sources
- [x] Use the Agent Skills open specification as the canonical skill format:
  skill directory containing `SKILL.md` with YAML frontmatter and Markdown body.
  Required fields are `name` and `description`; preserve optional fields such as
  `license`, `compatibility`, `metadata`, and `allowed-tools`.
- [x] Use a typed source value, for example `skill.Source`, with stable fields
  for ID/name, label, filesystem/root, and precedence/order.
- [x] Replace `SkillRoots` string inventory with typed skill sources. Bundles
  should contribute source roots, not individual skill directory strings.
- [x] Add a concrete skill repository implementation. Do not wrap the existing
  prototype interfaces if they do not match the intended repository model.
- [x] Extend `agent.Spec` so code-defined and filesystem-defined agents can
  configure skill sources in addition to named default skills.
- [x] Keep default pwd/home discovery in `app`, not hidden inside `agent.New`.
  Direct `agent.New` may load only sources provided explicitly through
  `agent.Spec` or `agent.Option`.
- [x] Define the app-to-agent skill handoff: app resolves default/bundle/plugin
  sources, combines them with spec sources, builds a `skill.Repository`, then
  passes that repository into agent construction.
- [x] Prefer passing a resolved `skill.Repository` into `agent.Instance` when
  app constructs agents. Direct `agent.New` may still accept explicit sources
  for simple single-agent SDK usage.
- [x] Add app/default skill source discovery for the final miniagent behavior:
  ```text
  <pwd>/.agents/skills
  <pwd>/.claude/skills
  ~/.agents/skills
  ~/.claude/skills
  ```
- [x] Treat `.agents/skills` as an Agent Skills compatibility source only. This
  does not imply support for `.agents/agents` or `.agents/commands` as default
  source layouts.
- [x] Define "pwd" precisely as the app workspace/current working directory used
  for this app run, and make home/workspace injectable for deterministic tests.
- [x] Keep missing default skill source directories non-fatal.
- [x] Add `.claude/skills` discovery to project/user source loading and
  `skills/` discovery to plugin root source loading.
- [x] When an `agent.Instance` is initialized, build its skill repo/list from:
  - typed app-provided default skill sources
  - typed loaded bundle/plugin skill sources
  - typed spec-defined skill sources
  - spec-named default skills
- [x] First iteration behavior: loaded default skills are materialized into the
  agent's system context deterministically before runtime engine construction.
- [x] Define deterministic materialization format and ordering:
  - base agent system prompt first
  - loaded skill sections after it
  - stable source order
  - stable skill name order within each source when source order alone is not
    enough
- [x] Define duplicate skill-name behavior. Default should be an explicit error
  unless a later manifest/priority rule says otherwise.
- [x] Defer dynamic skill retrieval/tools until after the miniagent equivalence
  proof. Do not design the first pass around a future dynamic mechanism.
- [x] Make loaded skill state inspectable for tests and frontends.
- [x] Expose a small app/agent API for resolved skill state, for example:
  - list available skills
  - list loaded skills
  - inspect which sources contributed them
- [x] Keep repository APIs read-oriented for frontends. Loading/unloading policy
  should remain explicit app/agent behavior, not implicit frontend behavior.
- [x] Add tests covering:
  - default source discovery from pwd `.agents/skills`
  - default source discovery from pwd `.claude/skills`
  - default source discovery from home `.agents/skills`
  - default source discovery from home `.claude/skills`
  - missing source directories are ignored
  - source ordering/precedence is deterministic
  - project source `.claude/agents`, `.claude/commands`, `.claude/skills`
  - plugin root source `agents`, `commands`, `skills`
  - embedded bundle skill sources
  - agent instance sees its resolved skill list/repository
  - loaded default skill content appears in the materialized system context
- [x] Current status: skills are loaded into `skill.Repository` and materialized
  into agent system context for default skills.

### Phase 8.1: SDK Runner API Cleanup

- [x] Add `agentdir.Resolution` helper methods so callers do not repeat bundle
  name enumeration and spec mutation:
  - `AgentNames`
  - `ResolveDefaultAgent`
  - `UpdateAgentSpec`
- [x] Add `agentdir.ResolveFS` for branded Go apps that embed an agent bundle
  but still want the same resource loading semantics as plugin roots.
- [x] Add app-level agent option helpers for common branded runner policy:
  workspace, tool timeout, session store, cache key prefix, verbose mode,
  output, and terminal UI.
- [x] Add `app.InstantiateDefaultAgent` so runners can resolve the selected
  agent once and then instantiate the app default.
- [x] Update `agentsdk run` to use the same resolver/default-skill/app-option
  APIs as branded runners.
- [x] Update miniagent to express only its product policy and resource location,
  not generic bundle agent-name plumbing or raw option threading.

### Phase 9: Clean Architecture Checkpoint

- [x] Remove new temporary aliases and compatibility shims introduced during
  agentsdk restructuring.
- [x] Remove terminal/repl app-command fallbacks and make app the only owner of
  command registration.
- [x] Verify all user input paths route through app-level APIs, including REPL
  and `agentsdk run` one-shot mode.
- [x] Verify app is the only owner of turn sequencing.
- [x] Replace stringly typed resource plumbing such as skill root strings with
  typed contribution values.
- [x] Verify default lookup paths follow external standards:
  - project/user skills from `.claude/skills`, `~/.claude/skills`,
    `.agents/skills`, and `~/.agents/skills`
  - project/user agents from `.claude/agents` and `~/.claude/agents`
  - project/user legacy commands from `.claude/commands` and
    `~/.claude/commands`
  - plugin resources from plugin root `agents`, `commands`, and `skills`
- [x] Verify plugin contribution interfaces are fully wired:
  - commands to command registry/scoped views
  - agent specs to spec registry
  - skills to typed skill sources
  - tools to app tool catalog
- [x] Confirm package names and public interfaces express ownership:
  - `runtime` owns execution engines
  - `agent` owns specs and running instances
  - `app` owns user routing, commands, sessions, loaded resources, and selected
    agents
  - `terminal/*` owns presentation only
- [x] Run `env GOCACHE=/tmp/go-cache go test ./...`.

### Phase 10: Miniagent Migration

- [x] Update miniagent to use agentsdk app/agent/terminal packages.
- [x] Move miniagent's primary coding agent definition into filesystem
  resources under `miniagent/.agents/agents`. This is intentionally loaded
  through explicit `agentsdk run <path>` plugin-source probing, not ambient
  project discovery.
- [x] Ensure miniagent's Go entry point loads those same filesystem resources
  instead of duplicating the agent definition in Go.
- [x] Embed miniagent's own `.agents` bundle into the branded binary so it does
  not depend on the process working directory and cannot accidentally load a
  user's project `.agents` directory as miniagent's primary bundle.
- [x] Keep miniagent-owned policy:
  - CLI flags and shell completions
  - default session directory `~/.miniagent/sessions`
  - default cache key prefix `miniagent:`
  - default coding agent spec/system prompt
  - benchmark/evolve machinery
- [x] Delete miniagent-owned generic mechanics and use direct SDK package usage.
  Do not introduce miniagent aliases/wrappers for SDK concepts:
  - runtime wrapper
  - session setup
  - tool setup
  - REPL loop
  - terminal display
  - command dispatch
- [ ] Convert miniagent workflow commands to Markdown command files under an
  agent bundle directory.
- [x] Target long-term shape:
  ```text
  miniagent/
    main.go
    .agents/
      agents/
        coder.md
      commands/
        review.md
        commit.md
        release.md
      skills/
        coder/
          SKILL.md
  ```
- [x] Keep `miniagent` as a branded app runner, while making the same bundle
  runnable by a generic SDK runner later:
  ```sh
  agentsdk run ../miniagent/agent
  ```
- [x] Add a proof test or smoke fixture showing the generic app loader and
  miniagent's `main.go` see the same loaded resources:
  - same primary agent name
  - same system prompt/spec
  - same Markdown commands
  - same skills
  - same default tool activation, unless miniagent adds Go-only extensions

### Phase 11: Generic SDK Runner Command

- [x] Add a CLI entry point for generic resource execution:
  ```sh
  agentsdk run <path> [task]
  ```
- [x] `agentsdk run` should create a built-in app, attach all resources found
  at the supplied path, choose a default agent, and then run one-shot or REPL
  mode.
- [x] Revisit `agentsdk run` after the app/skill cleanup so it uses the same
  app-level input routing and typed skill source APIs as miniagent.
- [x] Replace the current one-path direct `agentdir.LoadDir` behavior with the
  directory resolver from Phase 6.1.
- [x] Keep this initially narrow:
  - one path
  - one default or explicitly selected agent
  - terminal frontend only
  - no app manifest required
- [x] Support explicit agent selection:
  ```sh
  agentsdk run ../miniagent/agent --agent coder
  ```
- [ ] Later support multiple resource paths:
  ```sh
  agentsdk run ./base-agent ./team-plugin ./project-overrides --agent coder
  ```
- [ ] Later support remote plugin references from app manifests:
  ```text
  git
  http
  ```
- [ ] Define merge/priority rules for multi-path resources before enabling the
  multi-path mode.

### Phase 12: Terminal CLI Runner Extraction

Goal: remove the remaining generic app-runner boilerplate from miniagent while
keeping miniagent's product policy explicit. The SDK should own reusable
terminal CLI behavior; miniagent should provide resources and branded defaults.
The SDK CLI layer intentionally uses Cobra because the branded CLIs already use
it and Cobra gives us mature flag, help, and completion behavior.

- [x] Add a Cobra-backed SDK CLI package: `terminal/cli`.
- [x] Define a reusable run config that is still testable without invoking a
  shell command:
  ```go
  type Config struct {
      Resources      cli.Resources
      AgentName      string
      Task           string
      Workspace      string
      SessionsDir    string
      DefaultSessionsDir string
      Session        string
      ContinueLast   bool
      Inference      agent.InferenceOptions
      ApplyInference bool
      MaxSteps       int
      ApplyMaxSteps  bool
      SystemOverride string
      ToolTimeout    time.Duration
      TotalTimeout   time.Duration
      CacheKeyPrefix string
      Prompt         string
      In             io.Reader
      Out            io.Writer
      Err            io.Writer
  }
  ```
- [x] Move generic session helpers into the SDK CLI package:
  - resolve session by JSONL path
  - resolve session by id
  - resolve latest session for `--continue`
  - home-relative default session directory helper
- [x] Move generic one-shot/REPL execution into the SDK CLI runner:
  - resolve workspace
  - resolve default agent
  - apply inference/max-step/system CLI overrides to the selected spec
  - construct `app.App`
  - instantiate the default agent with optional resume session
  - route one-shot task through `app.Send`
  - route interactive mode through `terminal/repl`
  - apply total timeout and interrupt handling for one-shot mode
  - print final usage summary
  - translate `agent.ErrMaxStepsReached` into a warning and successful exit
- [x] Add resource constructors for branded and generic runners:
  ```go
  cli.DirResources(path string)
  cli.EmbeddedResources(fsys fs.FS, root string)
  cli.ResolvedResources(agentdir.Resolution)
  ```
- [x] Update `cmd/agentsdk run` to use the SDK CLI package. It no longer keeps
  a second hand-rolled app construction path.
- [x] Add focused runner tests for:
  - one-shot task execution
  - REPL path starts without a task
  - session id/path/continue resolution
  - selected agent spec override
  - default skill source discovery uses workspace
  - Cobra command execution with embedded resources
- [x] Add the Cobra command builder:
  ```go
  func NewCommand(config CommandConfig) *cobra.Command
  ```
  It should register standard flags for model/inference, workspace, sessions,
  timeout, tool timeout, max steps, system override, verbose mode, and shell
  completion.
- [x] Move model flag completion into the SDK CLI package. Initial behavior
  keeps the
  current static suggestions, but the SDK API should allow a pluggable completer
  so llmadapter/modeldb-backed completion can be added later.
- [x] Standardize positional task behavior in the SDK CLI package: all remaining
  positional arguments are joined into one task string.
- [ ] Switch miniagent embed pattern from agent-only files to the whole bundle
  before adding commands/skills:
  ```go
  //go:embed all:.agents
  ```
- [ ] Migrate miniagent `main.go` to a minimal branded app entry point:
  - embedded `.agents` resources
  - binary name/use/short/long text
  - default session dir `~/.miniagent/sessions`
  - cache prefix `miniagent:`
  - prompt `> `
  - any intentional miniagent-only Go extensions
- [ ] Remove miniagent-owned generic helpers after migration:
  - completion install helpers, if Cobra helper owns them
  - session path helpers
  - app construction
  - spec override plumbing
  - one-shot/REPL branching
- [ ] Add/keep miniagent tests proving:
  - embedded resources are used, not cwd `.agents`
  - miniagent and generic runner load equivalent bundle contents
  - branded defaults are applied
- [ ] Run verification:
  - `env GOCACHE=/tmp/go-cache go test ./...` in agentsdk
  - `env GOCACHE=/tmp/go-cache go test ./...` in miniagent
  - `agentsdk run ../miniagent --agent coder ...`
  - temporary built miniagent binary from outside the repo cwd

### Phase 13: Verification and Release Path

- [x] Run `go test ./...` in agentsdk after each phase.
- [ ] Update agentsdk README with the new package roles.
- [ ] Update miniagent AGENTS.md with the new architecture after migration.
- [x] Run `go test ./...` in miniagent after migration.
- [ ] Run `task install` in miniagent before smoke testing the installed binary.
- [ ] Smoke test:
  ```sh
  miniagent "say exactly: ok"
  miniagent --continue "say exactly: resumed"
  ```
- [ ] Only after review, commit/tag/release through the documented dependency
  chain.

## Design Decisions

- The user talks to an **app**, not directly to an agent.
- Commands are registered at the **app** level, even when contributed by agents
  or plugins.
- Frontends do not parse or execute commands. They submit user input to app
  routing and render app results/events.
- Agents may call commands only through explicitly exposed tools and policies.
- App owns turn sequencing across all user input surfaces.
- `runtime.Engine` should be the low-level execution primitive; an agent is an
  identity/config plus a running instance that uses an engine.
- miniagent is an app/distribution with one primary coding agent, not the owner
  of generic agent mechanics.
- Resource-described apps should be launchable by both a generic SDK runner and
  a branded Go `main.go`; when both load the same resources and no Go-only
  extensions are added, behavior should be equivalent.
- Skill sources are part of agent construction, not just bundle metadata. The
  final miniagent app and the generic SDK runner must resolve the same default
  skills from pwd and home `.claude/skills` and `.agents/skills` sources unless
  miniagent intentionally adds Go-only extensions.
- agentsdk should not invent a proprietary default resource directory while
  external conventions exist. `.claude/` is the default compatibility source for
  project/user agents and commands, `.agents/skills` is supported as an Agent
  Skills compatibility source, and plugin roots use Claude Code's `agents/`,
  `commands/`, and `skills/` layout.
- Default pwd/home skill discovery is app policy. `agent.New` must not silently
  inspect process-global pwd/home unless those sources were passed explicitly.
- Prefer the clean intended API over migration shortcuts. New compatibility
  adapters, fallback wrappers, or patch layers are out of scope unless they are
  explicitly approved for an external consumer migration.
- The first skill implementation should use deterministic static system-context
  materialization for loaded default skills. Dynamic skill loading/tooling is a
  later capability, not a prerequisite for miniagent migration.
- Default collision behavior for commands, agents, and skills is explicit error.
  Silent first-wins behavior is not acceptable for the clean architecture.
