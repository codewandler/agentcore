# Plan: Context and tooling improvements from session reviews

## Problem statement

The review notes in `.agents/reviews/` capture recurring friction from real
agent sessions: agents spend too many calls rediscovering repository state,
repeat shell workflows manually, lack safe native operations for common git and
filesystem writes, and cannot retrieve prior session history on demand. Some of
these are pure tool UX issues; others are better solved as compact context
providers that surface high-signal state before the model asks for it.

This plan consolidates those review-derived suggestions into an implementation
backlog for `agentsdk`, with explicit code targets and verification steps.

## Source review files

- `.agents/reviews/20260427T094200_plan-vs-implementation-architecture-review.md`
- `.agents/reviews/20260429T0415_tooling-improvement-brainstorm.md`
- `.agents/reviews/20260429T1200_command-registry-session-review.md`
- `.agents/reviews/20260430T0205_tool-use-and-preflight-context.md`
- `.agents/reviews/tool-use-issues.md`

Note: `.agents/reviews/20260427T094200_plan-vs-implementation-architecture-review.md`
reported the git context provider as missing at the time of that review. The
current tree already contains it in `agentcontext/contextproviders/git.go` and
`plugins/gitplugin/gitplugin.go`, so the current work is extension/hardening,
not a greenfield provider.

## Design principle

Separate improvements into two categories:

1. **Context providers** for compact, frequently useful, low-cardinality state
   that should be passively available at prompt boundaries.
2. **Tools** for high-cardinality, expensive, mutating, or task-specific data
   that should be retrieved or executed on demand.

This keeps prompt context small while improving the agent's ability to inspect
state and operate safely.

## Current implementation anchors

### Context/provider architecture

- `agentcontext/types.go` — provider/request/fragment types.
- `agentcontext/manager.go` — provider registration, render diffing,
  prepare/commit/rollback.
- `agentcontext/contextproviders/` — built-in providers.
- `runtime/thread_runtime.go` — context render persistence and injection.
- `app/plugin.go` — plugin facets, including context-provider facets.
- `plugins/*plugin/` — first-party multi-facet plugin implementations.

Existing relevant providers/plugins:

- `agentcontext/contextproviders/git.go`
- `plugins/gitplugin/gitplugin.go`
- `plugins/skillplugin/skillplugin.go`
- `plugins/toolmgmtplugin/toolmgmtplugin.go`

### Tool architecture

- `tool/` — core tool interface, typed tools, middleware, intent.
- `tools/filesystem/` — filesystem tools.
- `tools/git/` — git read tools.
- `tools/shell/` — bash tool.
- `capabilities/planner/` — stateful planner capability and plan tool.
- `tools/standard/standard.go` — default/catalog tool bundles.

## Priority roadmap

### Phase 1 — Small, high-confidence UX improvements

These are low-risk changes that directly address repeated review complaints.

#### 1. Improve planner single-plan UX

Current files:

- `capabilities/planner/actions.go`
- `capabilities/planner/tools.go`
- `capabilities/planner/*_test.go`

Current behavior:

```go
case ActionCreatePlan:
    if *created {
        return nil, fmt.Errorf("planner: plan already created")
    }
```

Problem:

- Reviews repeatedly hit `planner: plan already created`.
- The single-plan-per-planner-instance constraint is not obvious enough.
- The error does not tell the agent what plan exists or what to do next.

Implementation:

- Update the `create_plan` error to include active plan ID/title and suggested
  next action.
- Update tool guidance to explicitly say there is one active plan per planner
  instance.
- Keep the existing state model unchanged in this phase.

Example error:

```text
planner: plan already created (id="review-extract" title="Extract ..."); update the existing plan instead of calling create_plan again
```

Verification:

```bash
go test ./capabilities/planner/...
```

#### 2. Add line counts to `dir_tree`

Current files:

- `tools/filesystem/tools.go`
- `tools/filesystem/tools_test.go`
- `tools/filesystem/intent.go`

Add parameter:

```go
ShowLines bool `json:"show_lines,omitempty" jsonschema:"description=Show file line counts"`
```

Expected output when combined with `show_size`:

```text
runtime/thread_runtime.go (24KB, 784L)
tools/filesystem/tools.go (38KB, 1352L)
```

Rules:

- Count lines only for regular, non-binary files.
- Keep existing output unchanged when `show_lines=false`.
- Respect existing `max_entries`, depth, gitignore, flat/tree modes.

Verification:

```bash
go test ./tools/filesystem/...
```

#### 3. Add multi-command summaries to bash array output

Current files:

- `tools/shell/bash.go`
- `tools/shell/bash_test.go`

Problem:

- In command arrays, a later failure can obscure earlier successes.

Implementation:

For multiple commands, prepend a compact summary:

```text
Summary:
  ✓ command 1 exited 0 in 1.2s
  ✗ command 2 exited 1 in 0.1s
```

Keep existing detailed stdout/stderr blocks after the summary.

Verification:

```bash
go test ./tools/shell/...
```

#### 4. Improve `file_edit` guidance and diagnostics

Current files:

- `tools/filesystem/edit_impl.go`
- `tools/filesystem/edit_test.go`

Already implemented:

- Guidance says operations resolve against original content.
- Conflicts are detected.
- `replace` with `new_string:""` can delete text.

Remaining review-derived improvements:

- Make guidance more concrete:
  - deletion by exact text can use `replace` with empty `new_string`;
  - avoid batching dependent edits to the same region;
  - multi-file paths apply the same operation array to every matched file.
- Improve conflict errors with operation indexes and original line ranges where
  practical.
- Improve `old_string not found` errors with a suggestion to re-read the target
  region.

Verification:

```bash
go test ./tools/filesystem/...
```

### Phase 2 — High-value tool extensions

#### 5. Add native git write tools

Current files:

- `tools/git/git.go`
- `tools/git/intent.go`
- `tools/git/git_test.go`
- `plugins/gitplugin/gitplugin_test.go`
- `tools/standard/standard_test.go` if catalog/default expectations change

Current tools:

- `git_status`
- `git_diff`

Add tools:

```go
git_add
git_commit
git_reset
git_restore
```

Suggested parameter types:

```go
type GitAddParams struct {
    Paths []string `json:"paths" jsonschema:"required"`
}

type GitCommitParams struct {
    Message string   `json:"message" jsonschema:"required"`
    Add     []string `json:"add,omitempty"`
}

type GitResetParams struct {
    Mode string `json:"mode,omitempty"` // soft, mixed; no hard in first version
    Ref  string `json:"ref,omitempty"`
}

type GitRestoreParams struct {
    Paths  []string `json:"paths" jsonschema:"required"`
    Staged bool     `json:"staged,omitempty"`
}
```

Safety rules:

- Do not implement `git reset --hard` initially.
- Do not implement `git clean -fd` initially.
- `git_add` requires explicit paths.
- `git_commit` with `Add` stages only explicit paths.
- `git_commit` should return a staged-file summary before/after the commit.
- Destructive or history-rewriting operations should have conservative intents.

Verification:

```bash
go test ./tools/git/...
go test ./plugins/gitplugin/...
go test ./tools/standard/...
```

#### 6. Add native filesystem copy/move/directory-create tools

Current files:

- `tools/filesystem/tools.go`
- `tools/filesystem/intent.go`
- `tools/filesystem/tools_test.go`

Add tools:

```go
dir_create
file_copy
file_move
```

Suggested parameter types:

```go
type DirCreateParams struct {
    Path    string `json:"path" jsonschema:"required"`
    Parents bool   `json:"parents,omitempty"`
}

type FileCopyParams struct {
    Src       string `json:"src" jsonschema:"required"`
    Dst       string `json:"dst" jsonschema:"required"`
    Recursive bool   `json:"recursive,omitempty"`
    Overwrite bool   `json:"overwrite,omitempty"`
}

type FileMoveParams struct {
    Src       string `json:"src" jsonschema:"required"`
    Dst       string `json:"dst" jsonschema:"required"`
    Overwrite bool   `json:"overwrite,omitempty"`
}
```

Safety rules:

- `file_copy` refuses directories unless `recursive=true`.
- `file_copy` and `file_move` refuse overwrite unless `overwrite=true`.
- Be conservative around symlinks; document whether they are copied as links or
  followed.
- Recursive copy should return a count of files/directories copied.

Verification:

```bash
go test ./tools/filesystem/...
```

#### 7. Add `json_query` tool

Suggested new package:

- `tools/jsonquery/`

Problem:

- Reviews show repeated shelling out to Python for JSON analysis.

Suggested API:

```text
json_query(path="results.json", expr=".candidates[].features.parser")
```

Design options:

- Use a jq-compatible dependency such as `gojq`.
- Or implement a smaller JSONPath-like subset to avoid dependency weight.

Required behavior:

- Support normal JSON files.
- Consider JSONL or multi-document JSON as a follow-up.
- Truncate large results with counts.
- Return clear parse/query errors as tool error results.

Verification:

```bash
go test ./tools/jsonquery/...
```

### Phase 3 — Context-provider improvements

#### 8. Extend existing Git context provider with summary mode

Current files:

- `agentcontext/contextproviders/git.go`
- `agentcontext/contextproviders/git_test.go`
- `plugins/gitplugin/gitplugin.go`
- `plugins/gitplugin/gitplugin_test.go`

Current modes:

```go
GitOff
GitMinimal
GitChangedFiles
```

Add:

```go
GitSummary GitMode = "summary"
```

Summary mode should include:

- root
- branch
- head
- dirty
- staged count
- unstaged count
- untracked count
- changed-file count
- optional ahead/behind when cheap and available

Implementation notes:

- Replace or supplement `parseGitStatus(status string) []string` with a small
  structured status parser.
- Degrade gracefully when upstream is unavailable.
- Continue enforcing `MaxFiles` and `MaxBytes` caps.

Verification:

```bash
go test ./agentcontext/contextproviders/...
go test ./plugins/gitplugin/...
```

#### 9. Add project inventory context provider

Suggested new file:

- `agentcontext/contextproviders/project_inventory.go`

Purpose:

Expose compact file inventory with sizes and optional line counts so agents do
not spend tool calls rediscovering repository shape.

Possible API:

```go
type ProjectInventoryOption func(*ProjectInventoryProvider)

func ProjectInventory(opts ...ProjectInventoryOption) *ProjectInventoryProvider
func WithProjectInventoryRoot(root string) ProjectInventoryOption
func WithProjectInventoryMaxFiles(n int) ProjectInventoryOption
func WithProjectInventoryIncludeLines(enabled bool) ProjectInventoryOption
func WithProjectInventoryPatterns(patterns ...string) ProjectInventoryOption
```

Example fragment:

```text
significant_files:
  runtime/thread_runtime.go (784L, 24KB)
  tools/filesystem/tools.go (1352L, 38KB)
  tools/git/git.go (184L, 5KB)
```

Fragment key:

```text
project/inventory
```

Design rules:

- Respect `.gitignore` by default.
- Skip binary files.
- Cap file count and rendered bytes.
- Stable sort order.
- Stable fingerprint when file inventory has not changed.

Potential helper extraction:

- If needed, move reusable file metadata helpers from `tools/filesystem` into
  `internal/fsmeta` to avoid duplicating line counting and binary detection.

Verification:

```bash
go test ./agentcontext/contextproviders/...
```

#### 10. Add workflow suggestions provider and workflow tool

Suggested packages:

- `tools/workflow/`
- `plugins/workflowplugin/`
- possibly `agentcontext/contextproviders/workflows.go`

Problem:

- Reviews show repeated command sequences like build/vet/test loops.

Tool API:

```text
workflow(action="create", name="verify", steps=[...], failfast=true)
workflow(action="run", name="verify")
workflow(action="list")
workflow(action="delete", name="verify")
```

Provider output:

```text
suggested_workflows:
  verify-competition (seen 12 times, last 2m ago)
    cd competition && go build ./...
    cd competition && go vet ./...
```

Architecture:

- Use `tool.Middleware` to observe repeated `bash` invocations.
- Package this as `plugins/workflowplugin`, using `ToolMiddlewarePlugin` and
  context-provider facets.
- Keep workflow state session-scoped initially; do not write repo files like
  `Taskfile.yaml` automatically.

Verification:

```bash
go test ./tools/workflow/...
go test ./plugins/workflowplugin/...
```

### Phase 4 — On-demand retrieval and code intelligence

#### 11. Add conversation/session retrieval tool

Suggested package:

- `tools/conversation/`

Problem:

- Agents cannot efficiently search prior session messages/tool calls after
  compaction or long sessions.

Tool API:

```text
conversation(action="stats")
conversation(action="search", query="file_write competition/")
conversation(action="fetch", range=[-20, -10])
conversation(action="tool_calls", tool="bash", failed=true)
```

Integration approach:

- Add a runtime-provided searcher interface through `tool.Ctx.Extra()`.
- Follow existing patterns used by `tools/skills` and `tools/toolmgmt` for
  state lookup from extras.

Sketch:

```go
const KeyConversationSearch = "agentsdk.conversation.search"

type Searcher interface {
    Stats(ctx context.Context) (Stats, error)
    Search(ctx context.Context, query string, opts SearchOptions) ([]Match, error)
    Fetch(ctx context.Context, r Range) ([]Message, error)
}
```

Open design decision:

- Start with current runtime history for easy implementation, or use durable
  thread events for compaction/resume-safe retrieval.

Recommendation:

- Start with history/session stats and recent fetch.
- Add thread-event-backed search as a follow-up.

Verification:

```bash
go test ./tools/conversation/...
go test ./runtime/...
```

#### 12. Add symbol tree tool

Suggested package:

- `tools/symbols/`

Problem:

- Agents over-read large files when only function/type locations are needed.

Tool API:

```text
symbol_tree(path="runtime/thread_runtime.go")
symbol_tree(path=".", language="go", max_files=50)
```

Initial Go output:

```text
runtime/thread_runtime.go (784L)
  types: ThreadRuntime:35 TurnConfig:491
  funcs: ResumeThreadRuntime:96 contextInjectionForRender:593
  methods:
    ThreadRuntime.PrepareRequest:242
    ThreadRuntime.Compact:272
```

Implementation:

- Use `go/parser`, `go/ast`, and `token.FileSet`.
- Make this an optional/catalog tool initially, not necessarily default.

Verification:

```bash
go test ./tools/symbols/...
```

#### 13. Add Go profile helper tool

Suggested package:

- `tools/goprofile/` or `tools/profile/`

Tool API:

```text
go_profile(command="go test -bench=BenchmarkX -cpuprofile cpu.out ./pkg", top=20)
```

Behavior:

- Run benchmark/profile command.
- Run `go tool pprof -top`.
- Return top-N hot functions with cumulative percentages.

Design note:

- Keep this optional because it is Go-specific and shell-dependent.

Verification:

```bash
go test ./tools/goprofile/...
```

### Phase 5 — Larger architecture hardening

#### 14. Add typed `HarnessState` to context requests

Current file:

- `agentcontext/types.go`

Current issue:

- `Request.HarnessState` is `any`.

Future shape:

```go
type HarnessState struct {
    Model       ModelState       `json:"model,omitempty"`
    Environment EnvironmentState `json:"environment,omitempty"`
    Tools       ToolState        `json:"tools,omitempty"`
    Skills      SkillState       `json:"skills,omitempty"`
    Permissions PermissionState  `json:"permissions,omitempty"`
}
```

Recommendation:

- Defer until a provider actually needs cross-cutting typed state.
- Avoid speculative churn while existing providers remain self-contained.

Verification when implemented:

```bash
go test ./agentcontext/... ./runtime/...
```

#### 15. Add plugin fragment authority policy

Current files:

- `agentcontext/manager.go`
- `agentcontext/types.go`
- `app/plugin.go`

Problem:

- Any provider can currently emit any authority.
- Third-party plugins should probably default to user authority unless granted
  developer authority explicitly.

Possible API:

```go
type AuthorityPolicy struct {
    Default FragmentAuthority
    Grants  map[ProviderKey][]FragmentAuthority
}

func WithAuthorityPolicy(policy AuthorityPolicy) ManagerOption
```

Enforcement point:

- `agentcontext.Manager.buildProviderRecord`

Recommendation:

- Return an error for disallowed authority rather than silently downgrading.

Verification:

```bash
go test ./agentcontext/...
```

#### 16. Add durable harness state-change events

Current areas:

- `runtime/thread_runtime.go`
- `thread/event.go`
- `thread/event_registry.go`

Potential events:

```text
harness.model_changed
harness.environment_changed
harness.skill_loaded
harness.skill_unloaded
harness.tool_activated
harness.tool_deactivated
```

Recommendation:

- Keep low priority until a consumer needs an event-level audit trail.
- Current context snapshots already capture the rendered effect.

## Cross-cutting tool UX improvements

### `multi_tool_use.parallel` duplicate-call warnings

Review issue:

- Agents accidentally batch identical calls or dependent calls in parallel.

Suggested behavior:

- Warn or reject identical tool invocations in one parallel batch.
- Document that parallel batching should be read-only and independent.
- Consider rejecting state-mutating tools in parallel unless explicitly allowed.

This likely belongs outside this repository if `multi_tool_use.parallel` is a
host/runtime-provided tool, but it is recorded here because it repeatedly affects
agentsdk usage.

### Background bash jobs

Current file:

- `tools/shell/bash.go`

Potential future API:

```text
bash(cmd="go test -bench=. ./...", background=true)
bash_status(job_id="...")
bash_cancel(job_id="...")
```

Recommendation:

- Do not implement this as a quick patch. It needs a session-scoped process
  registry, cleanup semantics, output buffering, and cancellation behavior.
- Prefer smaller bash improvements first: summaries and configurable max
  timeout.

## Documentation updates

When implementing provider/resource layout changes, follow `AGENTS.md` guidance
and update:

- `docs/RESOURCES.md` when adding filesystem resource layouts or compatibility
  formats.
- `README.md` if new public tools/providers are added to standard bundles.
- `CHANGELOG.md` for user-visible tool/provider additions.
- Relevant examples under `examples/` when SDK APIs or standard bundles change.

## Verification strategy

For focused changes, run package tests:

```bash
go test ./capabilities/planner/...
go test ./tools/filesystem/...
go test ./tools/git/...
go test ./tools/shell/...
go test ./agentcontext/contextproviders/...
go test ./plugins/...
```

Before committing a batch, run the full suite:

```bash
go test ./...
```

## Suggested first implementation batch

Start with these four changes because they are scoped, low-risk, and directly
supported by multiple reviews:

1. Improve planner single-plan guidance/error.
2. Add `show_lines` to `dir_tree`.
3. Add bash multi-command summaries.
4. Add `GitSummary` mode to the existing git context provider.

Then move to native write operations:

5. Add `git_add` and `git_commit` first; defer `git_reset` and `git_restore` if
   the safety surface is too large for the first patch.
6. Add `dir_create`, `file_copy`, and `file_move`.

## Open decisions

1. Should `GitSummary` be the default git context mode, or should the plugin
   continue defaulting to `GitMinimal`?
2. Should `git_commit` be allowed in the default toolset, or only in the catalog
   / optional git plugin?
3. Should `dir_tree.show_lines` count lines lazily with caps, or can it scan all
   listed files up to `max_entries`?
4. Should conversation search be backed by in-memory history first or directly
   by durable thread events?
5. Should workflow suggestions be purely session-scoped, or should users be able
   to persist them into project resources later?
6. Should plugin authority policy reject disallowed authority or downgrade it?
   This plan recommends rejection.
