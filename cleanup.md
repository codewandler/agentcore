# Cleanup / Restructuring Plan

- **Goal**
  - Stop feature work while cleanup is active.
  - Remove ownership drift instead of labeling it transitional.
  - Reduce boilerplate and hidden defaults instead of adding more seams.
  - Keep code aligned with:
    - `toolactivation.Manager` = mutable tool registry / activation state
    - `agent.Instance` = lifecycle façade, not a growing god object
    - `harness.Session` = session/channel boundary
    - `command.Result` = structured result, rendered at boundaries

- **Phase 1 — Fix tool ownership drift** ✅
  - Completed in:
    - `a56d9ca Move agent tool registry out of standard toolset`
    - `1e54f70 Rename tool activation package`
    - `30f0ac9 Make agent standard tools explicit`
  - Current state:
    - `agent.Instance` owns `toolActivation *toolactivation.Manager`.
    - `agent.WithTools(...)` initializes `toolactivation.New(tools...)` directly.
    - `tools/standard` has since been deleted; named plugins own composition.
    - `agent.New` no longer imports or silently installs generic tool bundles.
    - Hosts pass tools explicitly.

- **Phase 2 — Re-evaluate late-registration APIs after ownership fix** ✅
  - Completed in:
    - `2e66493 Narrow agent projection registration seams`
  - Current state:
    - `agent.Instance.RegisterTools(...)` remains, backed by `toolactivation.Manager`.
    - `agent.Instance.RegisterContextProviders(...)` remains for session projection attachment.
    - `runtime.Engine.RegisterTools(...)` was removed.
    - `runtime.Engine.RegisterContextProviders(...)` remains because runtime owns the active context manager for future turns.
    - Projection attachment has no generic standard-bundle knowledge.

- **Phase 3 — Reduce command/rendering boilerplate** ✅
  - Completed in:
    - `de839ec Add structured command notice payloads`
  - Current state:
    - repeated workflow command notices use structured `command.Notice`, `command.NotFound`, and `command.Unavailable` payloads.
    - Rendering remains centralized through `command.Render(...)` / payload display behavior.

- **Phase 4 — Fix one-shot terminal result discard** ✅
  - Completed in:
    - `a625066 Render one-shot harness command results`
  - Current state:
    - `terminal/cli/run.go` renders returned `command.Result` values for one-shot slash commands.
    - Normal streamed agent-turn behavior remains unchanged.

- **Phase 5 — Decide auto-attachment policy for command projection** ✅
  - Completed in:
    - `01d8409 Attach command projection to harness sessions`
  - Current state:
    - default harness sessions attach the session command projection.
    - attachment remains explicit/idempotent at the session seam.
    - agent-callable command policy still filters unsafe commands.

- **Phase 6 — Clean docs to match actual architecture** ✅
  - Completed in:
    - `7f2425f Document cleaned ownership boundaries`
  - Current state:
    - docs capture ownership guardrails for `toolactivation`, named plugin composition, command rendering, projections, and plugin/contribution invariants.

- **Optional deeper cleanup completed so far**
  - Removed concrete `tools/skills` and `tools/toolmgmt` imports from `runtime`.
  - Renamed generic `activation` package to explicit `toolactivation`.
  - Moved terminal event rendering out of `agent` and into `terminal/ui`.
  - Removed hidden standard-tool defaults from `app.New` and `agent.New`.
  - Deleted generic `tools/standard` and `plugins/standard`; local terminal composition now lives in `plugins/localcli`.
  - Added context-aware `app.PluginFactory` so hosts can resolve named plugin refs from `context.Context` plus config without introducing a separate profile system.

- **Remaining cleanup candidates**
  - Revisit `agent.Instance` responsibilities and move outward only where the slice deletes or simplifies more than it adds:
    - session lifecycle
    - context provider lifecycle
    - capability registry/session ownership
    - workflow recording
  - Revisit payload display design:
    - consider renderer registry only if it reduces code
    - do not add a registry if payload `Display(...)` is currently simpler
  - Revisit `terminal/cli.Load`:
    - move shared resource/app/session setup toward harness loading helpers if it deletes duplication
    - keep terminal as the channel boundary
  - Revisit `app.App` workflow helper seams:
    - keep app as registry/composition host
    - move lifecycle-heavy workflow/session ownership toward harness when there is a concrete replacement path

- **Guardrails for any next slice**
  - No new harness plugin system beside `app.Plugin`.
  - No new command switch namespaces; use declarative `command.Tree`.
  - No generic `tools/standard` or `plugins/standard` default composition packages.
  - No separate profile system for plugin composition; named composition is still `app.Plugin` plus `app.PluginFactory`.
  - No hidden default tool bundles in `app.New` or `agent.New`.
  - No command output discarded at terminal/channel boundaries.
  - New seams should delete or collapse an old path.
  - Commit only after focused and full verification pass.
