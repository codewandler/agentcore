# Plan: Skill discovery visibility, runtime activation, and reference activation

## Problem statement

The current skills flow in `agentsdk` supports:
- discovery of skills from configured sources
- loading spec-declared skills into the system prompt
- a skills context provider that renders loaded skills

What is missing is explicit runtime skill activation.

We want the agent and the human operator to be able to:
- see all discovered skills that are eligible for activation
- see which skills are already activated
- activate additional skills during a session
- persist those activations across resumed sessions
- activate specific skill references in addition to whole skills
- expose the same core behavior via a model tool and user commands

The design should also account for the existing concept of skill references and for the `WhenEntry.Refs` metadata that already exists in `skill/metadata.go`.

## Confirmed requirements

1. If the agent has the skill tool, it may activate skills at runtime.
2. Activated skills should persist across resumed sessions.
3. The skills context provider should show discovered skills and their current status.
4. Add user commands:
   - `/skills` — list all skills and their activation/load status
   - `/skill <name>` — activate the skill
5. References need activation semantics too.
6. The skill tool should likely support batched actions, not only a single skill load.
7. New code must use `agentsdk` naming; do not introduce new `flai` names.
8. If resource layout expectations change, update `docs/RESOURCES.md`.
9. Run `go test ./...` before considering the work complete.

## Final decisions locked for phase 1

The following questions are now decided and should be treated as implementation constraints, not open questions:

1. **Reference scope**
   - Only files under the skill-local `references/` directory are eligible for reference activation.
   - Reference paths are exact relative paths only.
   - Arrays of reference paths are allowed.
   - Validation must reject paths outside `references/`, path traversal attempts, absolute paths, and `SKILL.md`.

2. **`/skill <name>` target**
   - The command operates on the current instantiated agent.
   - If there is no current/default instantiated agent, `/skill <name>` fails with a clear message.

3. **Command/tool policy**
   - `/skill <name>` is allowed regardless of whether the agent has the `skill` tool.
   - The `skill` tool itself remains model-gated by whether the agent exposes that tool.

4. **Baseline vs dynamic markers**
   - `/skills` should mark spec/baseline-activated skills differently from runtime-activated skills.
   - Keep the marker lightweight in phase 1.

5. **Replay behavior**
   - Resume replay is best-effort and non-fatal.
   - Favor observability: replay mismatches should produce diagnostics/warnings rather than silent skips.

6. **Reference rendering budget**
   - Available references render metadata only.
   - Activated references render metadata plus content.

7. **Reference visibility in context**
   - Only activated references are shown in the prompt-facing skills context provider.
   - Non-activated references remain discoverable through `/skills` and the tool/agent state APIs.

8. **Reference matching semantics**
   - Reference activation uses exact relative paths only in phase 1.
   - Globs/pattern expansion are explicitly deferred.

9. **Activation visibility timing**
   - Activation affects subsequent turns only.
   - The agent refreshes materialized skill state immediately after mutation so the next turn sees current state deterministically.

10. **Activation state structure**
    - Keep the discovered catalog immutable.
    - Introduce a separate mutable activation-state wrapper.

11. **Existing `SkillRepository()` compatibility**
    - Preserve the existing accessor for compatibility.
    - It may delegate internally or expose the underlying catalog, but existing callers should not break in phase 1.

12. **Replay failure policy**
    - Missing replayed skills/references are non-fatal.
    - Replay should continue and emit deterministic diagnostics.

13. **Context fragment structure**
    - Each activated skill and each activated reference should map naturally to its own file-like fragment where practical.
    - Fragment rendering should include path-like identity and relevant metadata such as source and, where available, modification timestamp.

14. **`/skills` without instantiated agent**
    - If there is no instantiated current agent, `/skills` should still show discovery results when a default spec/source set is known.
    - If there is a current agent, `/skills` should show that agent’s live activation state.

15. **Reference activation dependency**
    - References are only meaningfully activatable after their parent skill is active.
    - Phase 1 UX should require skill activation first rather than implicitly activating the parent skill from a reference-only request.

16. **Deferred automation**
    - `WhenEntry.Refs`, trigger-driven auto-activation, and other heuristic reference activation stay out of phase 1.
    - The plan should still describe how phase 2+ would build on the same activation model.

## Current state summary

### agentsdk today

Relevant findings from the current codebase:

- `skill.Repository` in `skill/skill.go` already supports:
  - discovery via `List()` / `Get()`
  - load state via `Load()` / `Loaded()` / `LoadedNames()`
- `skill.Repository` does **not** currently expose:
  - `IsLoaded(name)`
  - reference enumeration on `skill.Skill`
  - reference-specific load state
  - unload or reference unload APIs
  - persistence hooks for runtime activation changes
- `skill.Skill` currently stores:
  - `Name`, `Description`, `Metadata`, `SourceID`, `SourceLabel`, `Dir`, `Body`
  - but not discovered reference metadata
- `agentcontext/contextproviders/static.go` currently has `Skills(skills ...skill.Skill)` which only renders loaded skills with full body content.
- `agent.Instance.initSkills()` builds a repository once from spec skill sources and spec skill names.
- current materialization is based on `Repository.Materialize()` and therefore only sees loaded skill bodies.

### reference-related support already present

The `skill` package already has useful primitives:

- `SkillMetadata` with `When`, `Coder`, and `WhenEntry.Refs`
- `RefMetadata` with `Trigger`, `Triggers`, and `When`

This means the repository format already knows enough to support:
- listing references under a skill
- rendering reference metadata in discovery views
- explicit reference activation
- later auto-activation heuristics based on `when` and trigger metadata

### nearby flai repo reference

The nearby `~/projects/flai` repo shows a more complete prior direction:

- `core/skill/skill.go`
  - explicit registry abstraction
  - `IsLoaded`, `LoadPaths`, `UnloadPaths`, `LoadedPaths`
- `runtime/skill/skill.go`
  - runtime-level reference file representation
- `tools/skills/skills.go`
  - `skill_list`, `skill_load`, `skill_unload`
  - support for loading whole skills and specific paths

That prior design is useful as inspiration, but should not be copied mechanically. `agentsdk` should adopt only the parts that fit its simpler current repository and runtime architecture.

## Recommended architecture

### Core design principle

Create a single session-scoped activation state per running agent instance that owns:
- discovered skill catalog
- activated skills
- activated references per skill
- effective prompt materialization state
- replay/persistence of dynamic activation events

This activation state should be the single source of truth for:
- skill prompt materialization
- skills context provider rendering
- the skill tool
- `/skills`
- `/skill <name>`
- resumed session replay

Do **not** let commands, tools, and context provider each infer activation state independently from partial data.

## Component diagram

```text
Skill Sources
  (.agents/skills, embedded bundle skills, etc.)
        |
        v
+----------------------------+
| Skill Catalog / Repository  |
| - discovered skills         |
| - discovered references     |
| - frontmatter metadata      |
+----------------------------+
        |
        v
+-----------------------------------+
| Agent Skill Activation State      |
| session-scoped, mutable           |
| - active skills                   |
| - active refs per skill           |
| - baseline vs dynamic activation  |
+-----------------------------------+
       |                |                 |
       |                |                 |
       v                v                 v
+--------------+  +----------------+  +------------------+
| Materializer |  | Context        |  | Persistence /    |
| system text  |  | provider       |  | replay events     |
+--------------+  +----------------+  +------------------+
       ^                ^
       |                |
       +--------+-------+
                |
     +-----------------------+
     | Mutators / Readers     |
     | - skill tool           |
     | - /skills              |
     | - /skill <name>        |
     +-----------------------+
```

## State model

Introduce a skill activation subsystem with these concepts:

### 1. Discovered catalog
Immutable for the lifetime of the running agent instance unless a deliberate refresh is added later.

Contains:
- all discovered skills
- skill frontmatter metadata
- discovered references for each skill
- reference frontmatter metadata
- source identity / label

### 2. Baseline activation
Set of skills active because the agent spec declared them.

Contains:
- spec-provided skills
- optionally spec-provided default references if later needed

### 3. Dynamic activation
State changed at runtime via commands or tools.

Contains:
- additionally activated skills
- additionally activated reference paths per skill

### 4. Effective activation
Computed union used for prompt materialization and context rendering.

Contains:
- active skills = baseline + dynamic skills
- active refs = baseline refs + dynamic refs

### 5. Persistence state
Activation changes should be restorable on resume.

Recommended persistence model:
- persist activation events in thread/session history
- replay them after restoring the thread runtime

## Discovery and catalog design

### Extend `skill.Skill`
Current `skill.Skill` is not rich enough for reference-aware activation. Extend the catalog shape to expose discovered references.

Recommended additions:
- a reference descriptor type, e.g. `skill.Reference`
- reference fields:
  - `Path`
  - `Metadata RefMetadata`
  - `Source skill/source inherited context`
  - optional `Body` only if/when activated or materialized

Recommended catalog APIs:
- `List()` still returns all discovered skills
- `Get(name)` returns discovered skill
- add reference-aware lookups such as:
  - `ListReferences(skillName string)`
  - `GetReference(skillName, path string)`

### Why not scan filesystem on every activation?
Because:
- the repository already scans at startup
- activation should be deterministic and cheap
- persisted resume should restore against a stable in-memory catalog

Recommendation:
- scan once during agent init / resume reconstruction
- keep catalog in memory

## Context provider design

### Current behavior
Today `contextproviders.Skills(skills ...skill.Skill)` renders only loaded skills with body content.

### New behavior
Replace or extend this provider so it can render:
- discovered skills with status markers
- activated references per skill
- available references per skill, metadata-only

### Prompt-context rendering recommendation
For model context, prefer grouped sections:

```text
skills:

activated:
- architecture [skill]
  description: Evaluate and design software architecture with clear trade-off analysis.
  source: skills
- architecture/references/example.md [reference]
  skill: architecture
  status: activated

available:
- postgres-tuning [skill]
  description: Diagnose and tune PostgreSQL performance.
  source: workspace .agents/skills
- postgres-tuning/references/indexes.md [reference]
  skill: postgres-tuning
  status: available
  triggers: indexes, query plan
```

### Rendering rules
- activated skills:
  - include metadata
  - include full skill body content because they are effectively loaded
- available skills:
  - metadata only from frontmatter
  - do not include full body content
- activated references:
  - include metadata and full content
- available references:
  - metadata only
- deterministic ordering:
  - sort skills by name
  - sort references by relative path

### Why grouped sections instead of only flat status markers?
Trade-off:
- grouped sections help the model distinguish what it can already rely on versus what it may activate
- flat lists are shorter but less semantically clear

Recommendation:
- grouped rendering for prompt context
- flat list with status markers for `/skills`

## Tool design

### Recommendation: one `skill` tool with batched actions

Because you now want both skill and reference activation and potentially multiple operations in one call, the tool should be action-based rather than split into separate one-off tools.

Recommended request shape:

```json
{
  "actions": [
    {"action": "activate", "skill": "postgres-tuning"},
    {"action": "activate", "skill": "postgres-tuning", "references": ["references/indexes.md", "references/query-plan.md"]},
    {"action": "activate", "skill": "incident-review"}
  ]
}
```

### Phase 1 supported actions
- `activate`
- optionally `list` is unnecessary if `/skills` and context already cover discovery, but tool-side listing may still be useful later

### Deferred actions
- `deactivate`
- `refresh`
- `search`
- `install`

### Semantics of `activate`
Action payload:
- `skill` — required
- `references` — optional list of relative reference paths

Interpretation:
- if only `skill` is provided:
  - activate the skill
- if `references` are also provided:
  - activate the skill if not already active
  - activate those references for that skill

### Response shape
Return per-action and per-item results, e.g.

```json
{
  "results": [
    {
      "action": "activate",
      "skill": "postgres-tuning",
      "skill_status": "activated",
      "reference_results": [
        {"path": "references/indexes.md", "status": "activated"},
        {"path": "references/query-plan.md", "status": "already_activated"}
      ]
    },
    {
      "action": "activate",
      "skill": "incident-review",
      "skill_status": "already_activated"
    }
  ],
  "active_skills": ["architecture", "code-review", "incident-review", "postgres-tuning"]
}
```

### Validation rules
- empty `actions` → error
- unknown action → error
- missing `skill` on activate → error
- unknown skill → per-action failure result
- unknown reference path → per-reference failure result
- duplicate actions should be tolerated and produce `already_activated` where appropriate

### Why one batched tool instead of `skill_load`/`skill_load_paths`?
Pros:
- cleaner API for future growth
- supports multi-step activation in one tool call
- maps well to your new requirement

Cons:
- slightly more complex implementation than a single-purpose load tool

Recommendation:
- use one `skill` tool with `actions`

## Command design

### `/skills`
Human-facing listing of all discovered skills and references with status markers.

Recommended output format:

```text
skills:
- architecture [activated] — Evaluate and design software architecture with clear trade-off analysis.
  refs:
  - references/tradeoffs.md [available]
- postgres-tuning [available] — Diagnose and tune PostgreSQL performance.
  refs:
  - references/indexes.md [available]
  - references/query-plan.md [available]
```

### `/skill <name>`
Human-facing shortcut for activating a skill.

Recommended phase 1 behavior:
- activate whole skill only
- no explicit reference flags yet from command line
- returns:
  - loaded/activated
  - already activated
  - not found

Example:

```text
skill: activated "postgres-tuning"
```

### Optional future command
Deferred for now but compatible with the tool design:
- `/skill <name> --ref references/indexes.md`
- or `/skillref <skill> <path>`

Recommendation:
- do not add ref-specific commands in phase 1 unless a real UX need appears

## Reference activation design

### Why references need explicit activation
References are additional skill-local material that can expand context significantly. Treating them like separately activatable prompt fragments is the right abstraction.

### Recommended behavior
- activating a skill does **not** automatically activate all references
- references are opt-in unless later auto-load heuristics decide otherwise
- reference activation implicitly requires the parent skill to be active
- if a reference is activated for an inactive skill, activate the skill first

### Auto-activation hooks for later
The existing metadata model already hints at future heuristics:
- `SkillMetadata.When`
- `RefMetadata.When`
- `WhenEntry.Refs`

For this feature, do not fully implement heuristic auto-loading unless it comes almost for free.

Instead, design the activation subsystem so later you can add:
- startup-time detector-driven activation
- reference auto-activation after skill activation
- trigger-driven recommendation logic

### Phase 1 recommendation for `WhenEntry.Refs`
- parse and retain metadata in the catalog
- render it or keep it accessible for debugging/tests if useful
- do not make it automatic policy yet unless implementation turns out trivial and deterministic

## Persistence design

### Requirement
Activated skills must persist across resumed sessions.

### Recommended approach
Persist activation as runtime/thread events.

Event examples:
- `harness.skill_activated`
- `harness.skill_reference_activated`

Suggested payloads:

```json
{"skill":"postgres-tuning"}
```

```json
{"skill":"postgres-tuning","path":"references/indexes.md"}
```

### Replay behavior
On resume:
1. rebuild discovered catalog from configured sources
2. reconstruct baseline spec activation
3. replay dynamic activation events in original order
4. rebuild effective prompt materialization state

### Trade-off
Event replay vs blob snapshot:
- events are more verbose to implement
- but fit existing runtime replay patterns better and are auditable

Recommendation:
- use events

## Integration points in agentsdk

### `skill/skill.go`
Likely needs the largest expansion:
- add reference discovery model
- add activation-aware repository or new registry-like abstraction
- add `IsLoaded`-like checks
- add reference activation methods
- keep current deterministic ordering behavior

Potential API direction:
- `IsLoaded(name string) bool`
- `Load(name string) error`
- `LoadReferences(name string, paths []string) ([]string, error)`
- `LoadedReferences(name string) []Reference`
- `ListReferences(name string) []Reference`
- `Materialize()` should include loaded skill bodies and loaded reference bodies

### `agent/agent.go`
Needs to stop treating the repository as immutable startup-only state.

Likely changes:
- keep a mutable activation-aware skill state on `agent.Instance`
- expose methods used by tool/commands:
  - `ListSkills()`
  - `ActivateSkill(name string)`
  - `ActivateSkillReferences(name string, paths []string)`
- ensure materialized system text can refresh after activation changes
- ensure context provider sees refreshed state

### `agentcontext/contextproviders/static.go`
Refactor the skills provider:
- current helper only accepts loaded `skill.Skill`
- replace with a provider that can render both discovered and activated state
- likely better to make it consume a small interface rather than raw `[]skill.Skill`

### `runtime/thread_runtime.go` and related runtime replay code
Add event kinds and replay handlers for:
- skill activation
- reference activation

### `app/app.go`
Add built-in commands:
- `/skills`
- `/skill`

Behavior should call shared app/agent methods, not duplicate business logic.

### tool wiring
Add new `skill` tool package, probably under:
- `tools/skill/`

Follow the same pattern as `tools/toolmgmt`:
- fetch activation state object from `tool.Ctx.Extra()`
- return structured tool results
- keep user-facing result strings concise and actionable

## API shape recommendation

### Preferred package boundary
Create a focused runtime-facing activation abstraction in the `skill` package rather than a separate second package unless code size forces it.

Two viable options:

#### Option A: extend `skill.Repository`
Pros:
- minimal churn
- preserves current public surface shape

Cons:
- repository becomes both catalog and mutable runtime state owner
- naming may get fuzzy

#### Option B: keep `Repository` as catalog + add activation state wrapper
Example concept:
- `Repository` = discovered catalog
- `SessionState` or `ActivationState` = mutable active set and reference state

Pros:
- clearer separation of immutable discovery from mutable runtime state
- better fit for replay/persistence

Cons:
- a bit more code up front

### Recommendation
Prefer **Option B**.

Reason:
- your new feature introduces distinct mutable state and event replay concerns
- discovery catalog and runtime activation are different responsibilities
- this separation will keep future auto-activation logic cleaner

## Suggested implementation phases

### Phase 1: foundation
1. Inspect and finalize file-by-file design.
2. Add discovered reference model to `skill` package.
3. Introduce activation state wrapper around skill catalog.
4. Add materialization support for activated references.

### Phase 2: runtime wiring
5. Store activation state on `agent.Instance`.
6. Refresh materialized system/context after activation changes.
7. Add runtime events and replay for activation persistence.

### Phase 3: user and model interfaces
8. Add `tools/skill` with batched `actions` API.
9. Add `/skills` and `/skill <name>` commands.
10. Update skills context provider rendering.

### Phase 4: polish
11. Update docs:
    - `docs/RESOURCES.md` if reference/skill resource expectations become externally visible
    - `README.md` if public API/tooling behavior changes
    - `examples/engineer/README.md` and any other impacted example
12. Run `go test ./...`

## Tests to add

### `skill/...`
- discovery includes references
- deterministic reference ordering
- activating a skill marks it active
- activating refs marks only those refs active
- activating refs requires the parent skill to already be active
- activating unknown ref returns clear error
- materialization includes activated refs and excludes inactive refs

### `agentcontext/contextproviders/...`
- skills provider renders activated vs available sections
- available entries only show metadata
- activated refs include content
- deterministic ordering

### `agent/...`
- runtime activation updates materialized system text
- baseline spec skills remain active after replay
- resumed sessions restore dynamic skill activations
- resumed sessions restore activated references

### `tools/skill/...`
- empty actions rejected
- multiple activate actions handled in one call
- duplicate actions produce `already_activated`
- unknown skill/ref reported per action

### `app/...`
- `/skills` lists discovered skills with statuses
- `/skill <name>` activates a skill
- `/skill` with missing arg shows usage
- command output updates after activation

## Trade-offs considered

### One tool with actions vs multiple small tools
- one tool with actions is more extensible and aligns with reference activation
- multiple tools are simpler initially but become awkward once references enter the model

Recommendation: one `skill` tool with `actions`

### Auto-load all references on skill activation vs explicit reference activation
- auto-loading is simpler for users
- but can bloat context and hides cost

Recommendation: explicit reference activation by default

### Repository-only design vs separate activation state
- repository-only is less code initially
- separate activation state keeps responsibilities clearer and replay simpler

Recommendation: separate activation state wrapper

## Phase 2+ future extensions (deferred intentionally)

These are intentionally deferred beyond phase 1, but the phase 1 architecture should make them possible without a rewrite:

- automatic activation of references from `WhenEntry.Refs`
- trigger-driven skill/reference recommendation and activation
- detector-driven startup auto-activation from `SkillMetadata.When` and `RefMetadata.When`
- reference glob/pattern activation
- deactivate/unload support for skills and references
- richer `/skill` command syntax for explicit reference activation
- remote search/install integration tied directly into runtime activation

## Out of scope for this feature

- unload/deactivate support in commands
- full reference-specific CLI syntax
- remote search/install integration for skill activation
- heuristic auto-activation policy beyond storing the metadata hooks
- dynamic filesystem rescans during a running session
- switching activation state across multiple agent instances globally

## Risks and open questions

### Risks
1. Drift between materialized system text and context provider rendering
   - Mitigation: both must read from the same activation state.
2. Resume replay may fail if a previously activated skill/reference is missing from the current skill source.
   - Mitigation: replay should skip missing entries with a deterministic warning path or event note.
3. Prompt bloat if reference activation is too implicit.
   - Mitigation: keep reference activation explicit in phase 1.
4. API churn in `skill` package if we overfit to the old `flai` shape.
   - Mitigation: borrow concepts, not exact interfaces.

### Open questions to settle during implementation
1. Should a reference path be allowed outside `references/` or any non-`SKILL.md` file under the skill root?
   - Recommendation: allow any safe relative file under the skill root, but prefer `references/` convention in docs.
2. Should baseline spec skills be marked differently from dynamically activated skills in `/skills` output?
   - Recommendation: optional later; not required in phase 1.
3. How should replay surface missing skills/references from old sessions?
   - Recommendation: best-effort replay plus diagnostics, not fatal session restore.

## Verification plan

Implementation is complete when:
- runtime activation of skills works via shared agent state
- activated skills persist across resume
- references can be activated via tool actions
- `/skills` and `/skill <name>` work against the same state
- the context provider renders discovered and activated state correctly
- docs/examples are updated where behavior is externally visible
- all tests pass:

```bash
go test ./...
```


## Concrete execution checklist

Use this as the implementation order. Each checkbox should correspond to a small, reviewable commit or at least a coherent diff slice.

### Phase 0 — align scope before coding

- [x] Limit reference activation to exact relative paths under `references/` only.
- [x] Make `/skill <name>` operate on the current instantiated agent and fail clearly if none exists.
- [x] Mark baseline/spec-loaded skills differently from runtime-activated skills in phase 1.
- [x] Make replay best-effort and non-fatal, with observable diagnostics/warnings.
- [x] Show only activated references in prompt-facing context; keep available reference discovery in `/skills` and state APIs.
- [x] Defer `WhenEntry.Refs` automation and trigger-driven auto-activation to a later phase.

### Phase 1 — extend skill discovery model

Files likely involved:
- `skill/skill.go`
- `skill/ref.go`
- `skill/ref_test.go`
- `skill/skill_test.go`

Execution steps:
- [ ] Add a discovered reference model in `skill/`, e.g. `Reference` or `SkillReference`.
- [ ] Extend skill scanning to enumerate reference files for each discovered skill.
- [ ] Parse frontmatter for discovered references using the existing `RefMetadata` type.
- [ ] Store reference metadata in memory alongside each discovered skill.
- [ ] Add deterministic ordering for references by relative path.
- [ ] Add catalog APIs for listing/getting references by skill name.
- [ ] Keep current skill discovery behavior backward-compatible for callers that only care about whole skills.

Tests:
- [ ] Add tests that a skill with references is discovered correctly.
- [ ] Add tests that references without frontmatter still have stable discovery behavior if they should be activatable.
- [ ] Add tests for deterministic ordering across multiple references.
- [ ] Add tests for invalid reference paths being rejected: outside `references/`, absolute paths, traversal (`..`), empty path, and `SKILL.md`.

### Phase 2 — introduce runtime activation state

Files likely involved:
- `skill/skill.go` or new `skill/activation.go`
- `agent/agent.go`
- `agent/options.go` if wiring changes are needed
- `agent/agent_test.go`

Execution steps:
- [ ] Introduce a session-scoped activation state wrapper around the discovered skill catalog.
- [ ] Represent baseline active skills from the agent spec separately from dynamic activations.
- [ ] Track dynamically activated references per skill.
- [ ] Add APIs such as:
  - [ ] list discovered skills
  - [ ] list active skills
  - [ ] check whether a skill is active
  - [ ] activate a skill
  - [ ] activate references for a skill
  - [ ] list active references for a skill
- [ ] Enforce that reference activation requires the parent skill to already be active.
- [ ] Return a clear validation/result error when reference activation is requested for an inactive skill.
- [ ] Make state changes idempotent so repeated activate calls are safe.
- [ ] Ensure the activation state is the single source of truth used by both commands and tools.

Tests:
- [ ] Add tests for baseline spec skills being active at startup.
- [ ] Add tests for activating a new skill.
- [ ] Add tests for activating the same skill twice returning an idempotent result.
- [ ] Add tests for activating references on an already active skill.
- [ ] Add tests for activating references on an inactive skill returning a clear failure result.
- [ ] Add tests for unknown skill and unknown reference errors.

### Phase 3 — update prompt materialization

Files likely involved:
- `skill/skill.go`
- `agent/agent.go`
- any system-building helper touched by skill materialization

Execution steps:
- [ ] Refactor skill materialization so it reads from activation state, not only startup-loaded repository order.
- [ ] Ensure activated skills include their full body content in the materialized system prompt.
- [ ] Ensure activated references include their full content in the materialized system prompt.
- [ ] Ensure non-activated skills/references contribute metadata only via the context provider, not prompt body content.
- [ ] Make materialization deterministic after runtime activation changes.
- [ ] Provide a refresh/rebuild path after each activation mutation.

Tests:
- [ ] Add tests that runtime-activated skills appear in `MaterializedSystem()`.
- [ ] Add tests that activated references appear in `MaterializedSystem()`.
- [ ] Add tests that non-activated references do not appear in `MaterializedSystem()`.
- [ ] Add tests for deterministic output order.

### Phase 4 — refactor skills context provider

Files likely involved:
- `agentcontext/contextproviders/static.go`
- `agentcontext/contextproviders/static_test.go`
- possibly a new provider file if the current static helper becomes too generic

Execution steps:
- [ ] Replace or extend `contextproviders.Skills(...)` so it can render discovered + activated state.
- [ ] Render grouped sections for `activated` and `available`.
- [ ] Include status markers for skills and references.
- [ ] Include metadata-only output for available skills.
- [ ] Show only activated references in prompt-facing context.
- [ ] Include full content for activated skills and activated references.
- [ ] Keep fragment keys deterministic and stable.
- [ ] Render each activated skill and each activated reference as its own file-like fragment where practical.

Tests:
- [ ] Add tests for activated skill rendering.
- [ ] Add tests for available skill rendering without body content.
- [ ] Add tests that non-activated references are omitted from prompt-facing context.
- [ ] Add tests for activated reference rendering.
- [ ] Add tests for stable fragment keys.
- [ ] Add tests for deterministic ordering of skills and refs.

### Phase 5 — add persistence and replay

Files likely involved:
- `runtime/thread_runtime.go`
- `runtime/runtime.go`
- `agent/agent.go`
- thread/runtime tests near replay/event handling

Execution steps:
- [ ] Define event kinds for skill activation and reference activation.
- [ ] Emit a thread/runtime event whenever a skill is activated dynamically.
- [ ] Emit a thread/runtime event whenever references are activated dynamically.
- [ ] Add replay logic that restores dynamic activation state on resume.
- [ ] Rebuild effective activation/materialization state after replay.
- [ ] On replay of missing skills/references from changed sources, skip them non-fatally.
- [ ] Surface deterministic diagnostics for replay mismatches through an explicit diagnostics path.
- [ ] Decide the exact diagnostics sink and document it in code/tests: app diagnostics, runtime event, command warning section, or a combination.

Tests:
- [ ] Add tests that dynamically activated skills persist across resume.
- [ ] Add tests that dynamically activated references persist across resume.
- [ ] Add tests that replay preserves activation ordering where relevant.
- [ ] Add tests for missing replayed skills/references producing non-fatal diagnostics if that is the chosen behavior.

### Phase 6 — add the `skill` tool

Files likely involved:
- new package under `tools/skill/` or similar
- `agent` tool wiring code
- `tool`-level tests for result formatting and validation

Execution steps:
- [ ] Create a new `skill` tool package using `agentsdk` naming.
- [ ] Inject activation state into `tool.Ctx.Extra()` with a new `agentsdk` key.
- [ ] Define request schema with `actions`.
- [ ] Implement `activate` action for whole skills.
- [ ] Implement `activate` action with `references` for per-skill reference activation.
- [ ] Validate that `references` contains exact relative paths only and all paths stay under `references/`.
- [ ] Reject reference activation when the parent skill is not already active.
- [ ] Return structured per-action results plus summarized active state.
- [ ] Write concise human-readable tool result strings.
- [ ] Ensure the tool fails clearly on invalid input and reports partial success explicitly.

Tests:
- [ ] Add tests for empty `actions`.
- [ ] Add tests for unknown action values.
- [ ] Add tests for activating one skill.
- [ ] Add tests for activating multiple skills in one call.
- [ ] Add tests for activating references.
- [ ] Add tests for duplicate actions yielding `already_activated` style results.
- [ ] Add tests for unknown skill/reference reporting.

### Phase 7 — add app commands

Files likely involved:
- `app/app.go`
- `app/app_test.go`
- possibly command registration helpers if builtins grow too large

Execution steps:
- [ ] Add `/skills` builtin command.
- [ ] Add `/skill <name>` builtin command.
- [ ] Make `/skills` render discovered skills and references with status markers.
- [ ] Mark baseline/spec-loaded skills separately from runtime-activated skills.
- [ ] Make `/skill <name>` call the same shared activation path as the tool.
- [ ] Add helpful usage text when `/skill` is missing the name argument.
- [ ] Return a clear message when `/skill <name>` is used and no current/default instantiated agent exists.
- [ ] Make `/skills` fall back to discovery-only output when there is no instantiated current agent but a default spec/source set is known.
- [ ] Verify command behavior in the `examples/engineer` flow.

Tests:
- [ ] Add tests for `/skills` with no default agent.
- [ ] Add tests for `/skills` with no instantiated agent but known discovery sources.
- [ ] Add tests for `/skills` on an instantiated agent.
- [ ] Add tests for `/skill <name>` success.
- [ ] Add tests for `/skill <name>` already activated.
- [ ] Add tests for `/skill <name>` unknown skill.
- [ ] Add tests for `/skill <name>` failing clearly when no current/default instantiated agent exists.
- [ ] Add tests for `/skill` missing argument.

### Phase 8 — update examples and docs

Files likely involved:
- `docs/RESOURCES.md`
- `README.md`
- `examples/engineer/README.md`
- other example READMEs if the public behavior is relevant there

Execution steps:
- [ ] Update `docs/RESOURCES.md` if reference discovery/activation changes the externally visible resource interpretation.
- [ ] Update public README/docs if the new `skill` tool is part of the public SDK story.
- [ ] Update `examples/engineer/README.md` with `/skills` and `/skill` usage.
- [ ] Add at least one example of activating a skill and, if documented, activating explicit references under `references/`.
- [ ] Review whether `examples/devops-cli/` or `examples/research-desk/` should be updated to demonstrate the API if they are better showcases.

### Phase 9 — final verification and cleanup

Execution steps:
- [ ] Run focused package tests while iterating, e.g.:
  - [ ] `go test ./skill/...`
  - [ ] `go test ./agentcontext/...`
  - [ ] `go test ./agent/...`
  - [ ] `go test ./runtime/...`
  - [ ] `go test ./app/...`
- [ ] Run the full suite:
  - [ ] `go test ./...`
- [ ] Manually smoke test the engineer example:
  - [ ] `go run ./cmd/agentsdk run examples/engineer`
  - [ ] verify `/skills`
  - [ ] verify `/skill <name>`
  - [ ] verify activated skills persist after resume if resume flow is available
- [ ] Review diffs for accidental new `flai`-prefixed names in new code.
- [ ] Review whether docs and examples reflect the final API exactly.

## Suggested commit slices

If you want to keep reviewable commits, this is a good breakdown:

- [ ] Commit 1: skill catalog gains reference discovery + tests
- [ ] Commit 2: runtime activation state for skills/refs + tests
- [ ] Commit 3: prompt materialization + context provider rendering + tests
- [ ] Commit 4: persistence/replay events + tests
- [ ] Commit 5: `skill` tool + tests
- [ ] Commit 6: `/skills` and `/skill` commands + tests
- [ ] Commit 7: docs/examples updates
- [ ] Commit 8: final cleanup after full `go test ./...`

## Definition of done

This feature is done when all of the following are true:

- [ ] discovered skills and discovered references are visible without full activation
- [ ] runtime skill activation works
- [ ] runtime reference activation works
- [ ] activated skills affect the next turn’s prompt materialization
- [ ] activated references affect the next turn’s prompt materialization
- [ ] `/skills` reflects discovered and activated state
- [ ] `/skill <name>` activates through shared logic
- [ ] skill activation persists across resume
- [ ] tests cover the main success and failure paths
- [ ] `docs/RESOURCES.md` is updated if needed
- [ ] examples are updated if needed
- [ ] `go test ./...` passes
