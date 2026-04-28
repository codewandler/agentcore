# DESIGN: Conversation Compaction — `/compact` Command & Auto-Compaction

**Status:** Draft (rev 3 — final)  
**Date:** 2026-04-28  
**Author:** Agent-assisted design  

## 1. Problem Statement

agentsdk has a complete compaction infrastructure at the runtime layer:

- `conversation.CompactionEvent` — payload type with summary + replaced node IDs
- `conversation.ProjectItems` — replaces compacted nodes with summary messages at projection time
- `runtime.History.CompactContext` — appends a compaction event to the conversation tree
- `runtime.ThreadRuntime.Compact` — compacts + re-renders context with `PreferFull` / `RenderCompaction`
- `runtime.Engine.Compact` — delegates to ThreadRuntime or History

**What's missing:**

1. **No user-facing trigger.** There is no `/compact` slash command. The README
   references `agent.Compact(ctx, summary, nodeIDs...)` but this method does not
   exist on `agent.Instance`. Compaction is only reachable programmatically
   through `runtime.Engine.Compact`.

2. **No auto-compaction.** There is no mechanism to detect when a conversation
   is growing large and automatically trigger compaction. Long sessions
   accumulate unbounded history, increasing input token costs and eventually
   hitting context window limits.

3. **No `agent.Instance.Compact` method.** The agent layer has no compaction
   API, so neither a slash command nor auto-compaction can be wired without
   first adding this.

4. **No usage data in the thread event log.** Usage records (`usage.Record`)
   are tracked in-memory via `usage.Tracker` but never persisted to the thread
   event store. This means usage data is lost on session resume and cannot be
   audited from the event log.

5. **No model context window on the agent.** The agent does not know its
   model's context window size. `ModelResolutionCandidate` in llmadapter
   carries `Capabilities` but not `Limits`. Without the context window,
   auto-compaction must use an arbitrary fixed threshold instead of a
   model-aware percentage.

6. **No compaction floor on the tree.** After compaction, `tree.Path()` still
   walks from HEAD to root through all replaced nodes. With repeated
   compactions, projection cost grows with total historical nodes, not with
   the active conversation size. A floor pointer is needed to bound the
   traversal.

## 2. Existing Infrastructure Summary

```
Layer              What exists                              What's needed
─────────────────  ───────────────────────────────────────   ──────────────────────────
conversation       CompactionEvent, ProjectItems,           Compaction floor on Tree
                   compactionMessage, placeCompaction       (SetFloor, Path stops at floor)

runtime.History    CompactContext(ctx, summary, nodeIDs)     Set floor after compaction

runtime.Thread     Compact(ctx, history, summary, nodeIDs)   (complete)
Runtime            re-renders context after compaction

runtime.Engine     Compact(ctx, summary, nodeIDs)            (complete)
                   delegates to ThreadRuntime or History

agent.Instance     (nothing)                                 Compact method
                                                             auto-compaction hook
                                                             contextWindow field
                                                             usage event persistence

app.App            builtins() registers /help, /new, etc.    /compact command
                                                             auto-compaction config

command            Registry, Spec, Policy, Result types       (complete)

llmadapter         ModelResolutionCandidate has               Add Limits field
                   Capabilities but not Limits               Add ContextWindow to
                   AutoRouteSummary has no context window     AutoRouteSummary

usage              Tracker records in-memory only             Persist to thread events
```

## 3. Design: `/compact` Slash Command

### 3.1 Behavior

`/compact` triggers an explicit conversation compaction on the current session.
The flow:

1. User types `/compact`.
2. The command handler obtains the default agent instance.
3. It calls `inst.Compact(ctx)` (new method — see §6).
4. The agent asks the LLM to produce a summary of the conversation so far.
5. The agent calls `engine.Compact(ctx, summary, nodeIDs...)` with all
   non-compaction node IDs on the current branch except the most recent N
   messages (the "keep window").
6. The command returns `command.Text(...)` with a confirmation message showing
   before/after token estimates.

### 3.2 Command Spec

```go
command.Spec{
    Name:        "compact",
    Description: "Summarize and compact conversation history",
    Policy:      command.Policy{UserCallable: true},
}
```

- **User-callable:** Yes — this is a user-initiated action.
- **Agent-callable:** No — the agent should not compact its own context
  mid-turn. Auto-compaction (§4) handles the between-turns case.
- **Internal:** No.

### 3.3 Registration

The `/compact` command is registered as a **built-in** in `app.builtins()`,
alongside `/help`, `/new`, `/session`, `/context`, etc. It is always available
when an agent is running. No configuration flag is needed to enable it — it's a
core session management command like `/new`.

### 3.4 Summary Generation

Compaction requires a text summary of the replaced messages. Two strategies:

**Option A: LLM-generated summary (recommended)**

Run a dedicated summarization turn against the same client. This produces a
high-quality, semantically meaningful summary. The summary request uses a
focused system prompt:

```
Summarize the following conversation concisely. Preserve key decisions,
file paths, function names, and any state the assistant needs to continue
working. Output only the summary, no preamble.
```

The messages to summarize are the ones being replaced (everything except the
keep window). This is a single non-streaming request with a small max output
token limit (e.g. 1024).

Trade-off: costs one extra API call. For conversations large enough to need
compaction, this cost is negligible relative to the input tokens saved on
subsequent turns.

**Option B: Concatenated text extraction (fallback)**

Extract text content from replaced messages and truncate to a fixed character
limit. No API call needed, but produces lower-quality summaries.

**Decision:** Use Option A (LLM summary) as the primary path. Option B can
serve as a fallback if the summarization request fails (network error, rate
limit, etc.).

### 3.5 Keep Window

The "keep window" is the number of recent messages excluded from compaction.
These messages remain in the conversation tree as-is so the model retains
immediate context.

- **Default keep window:** 4 messages (typically the last user message, the
  last assistant response, and the preceding exchange).
- The keep window is not user-configurable in v1. It can be exposed later via
  `/compact --keep N` if needed.
- If the conversation has fewer messages than the keep window, `/compact`
  returns a message like "conversation too short to compact" and does nothing.

### 3.6 Node Selection

Which nodes to replace:

1. Walk `tree.Path(branch)` to get all nodes on the current branch.
2. Exclude the last `keepWindow` nodes.
3. Exclude any nodes that are already `CompactionEvent` payloads (don't
   re-compact summaries).
4. The remaining node IDs become the `replaces` list for `CompactionEvent`.

If the replaces list is empty after filtering, return early with a "nothing to
compact" message.

### 3.7 Output

After successful compaction, display:

```
Compacted conversation: replaced N messages with summary.
Estimated tokens: before=X after=Y (saved ~Z)
```

Token estimates use `conversation.EstimateMessagesTokens` on the projected
messages before and after compaction.

## 4. Design: Auto-Compaction

### 4.1 Trigger Point

Auto-compaction runs **between turns**, after the agent completes a turn and
before the next user input is processed. This is the natural point because:

- The conversation state is stable (no in-flight tool calls).
- The user is not waiting for a response yet.
- Context providers have been committed.

Specifically, auto-compaction is evaluated in `agent.Instance.RunTurn` after
the `runtime.Engine.RunTurn` call returns successfully.

### 4.2 Trigger Condition

Auto-compaction fires when the **estimated input token count** of the projected
conversation exceeds a configured threshold.

```go
type AutoCompactionConfig struct {
    Enabled        bool
    TokenThreshold int  // explicit override; 0 = use model-aware default
    KeepWindow     int  // messages to preserve, default 4
}
```

The token estimate is computed from the projected messages using
`conversation.EstimateMessagesTokens`. This is a cheap local computation (no
API call) — the same estimator already used by the projection policy.

**Why token threshold, not message count?**

Message count is a poor proxy for context pressure. A conversation with 10
messages containing large file contents may be larger than one with 50 short
exchanges. Token estimation directly measures what matters: how much of the
context window is consumed.

**Default threshold — model-aware via modeldb:**

When `TokenThreshold` is 0 (the default), the agent computes the threshold
automatically as **80% of the model's context window**:

```go
func (a *Instance) autoCompactionThreshold() int {
    if a.autoCompaction.TokenThreshold > 0 {
        return a.autoCompaction.TokenThreshold // explicit override
    }
    if a.contextWindow > 0 {
        return int(float64(a.contextWindow) * 0.8)
    }
    return defaultAutoCompactionThreshold // fallback: 80_000
}
```

The context window is resolved from modeldb at agent init time via the
`ModelResolutionCandidate.Limits` → `AutoRouteSummary.ContextWindow` chain
(see §4.7). This means:

- Claude Opus (200K context) → threshold ~160K
- GPT-4o (128K context) → threshold ~102K
- Claude Haiku (200K context) → threshold ~160K
- Unknown model (no modeldb entry) → fallback 80K

The 80% ratio leaves 20% headroom for system prompt, context fragments, tool
definitions, and the model's output tokens.

### 4.3 Auto-Compaction Flow

```
agent.RunTurn completes
  → estimate projected input tokens
  → if tokens < threshold: return normally
  → if tokens >= threshold:
      1. Generate summary via LLM (same as /compact, §3.4)
      2. Select nodes to replace (same as /compact, §3.6)
      3. Call engine.Compact(ctx, summary, nodeIDs...)
      4. Emit a runner event or log indicating auto-compaction occurred
      5. Return normally
```

### 4.4 Failure Handling

If auto-compaction fails (summary generation fails, compaction event fails):

- **Log the error** but do not fail the turn. The user's turn already completed
  successfully. Compaction is an optimization, not a correctness requirement.
- **Do not retry** in the same turn cycle. The next turn will re-evaluate the
  threshold and try again.

### 4.5 Configuration Surface

Auto-compaction is configured on `agent.Instance` via options:

```go
agent.WithAutoCompaction(agent.AutoCompactionConfig{
    Enabled:        true,
    TokenThreshold: 80_000,
    KeepWindow:     4,
})
```

For the app layer, this can be set via `app.WithAgentOptions(...)` or through
agent spec fields in resource bundles:

```yaml
# In an agent resource file
auto_compaction:
  enabled: true
  token_threshold: 80000
  keep_window: 4
```

### 4.6 Observability

When auto-compaction fires, emit a structured event so the terminal UI and
thread store can record it:

- Thread event kind: `"conversation.auto_compaction"` (distinct from the
  compaction event itself, which is `"conversation.compaction"`).
- Payload: `{ "trigger": "token_threshold", "estimated_tokens": N,
  "threshold": M, "context_window": W, "replaced_count": K }`.
- Terminal UI: print a dim status line like
  `[auto-compacted: replaced 12 messages, ~45K tokens saved]`.

### 4.7 Context Window Resolution via modeldb

The context window flows through the llmadapter model resolution pipeline:

```
modeldb.ModelRecord.Limits.ContextWindow (int, tokens)
  ↓ populated during
adapterconfig.ResolveModelCandidates
  ↓ stored on
ModelResolutionCandidate.Limits (new field: modeldb.Limits)
  ↓ surfaced via
AutoRouteSummary.ContextWindow (new field: int)
  ↓ consumed by
agent.Instance.resolveRouteIdentity → a.contextWindow
  ↓ used by
agent.Instance.autoCompactionThreshold → 0.8 * contextWindow
```

**Why this path is correct:**

1. **Single source of truth.** `modeldb.Limits` is the canonical source for
   model limits. It flows through the same resolution pipeline as capabilities.

2. **Every consumer benefits.** Any code that calls `ResolveModel` or
   `RouteSummary` gets limits for free — not just auto-compaction. Future
   features (cost estimation, context window warnings, model comparison) use
   the same path.

3. **No agentsdk→modeldb coupling for limits.** agentsdk reads an int from
   `AutoRouteSummary`, not from the modeldb catalog directly.

4. **Follows existing patterns.** `Capabilities` already flows through
   `ModelResolutionCandidate` the same way. `Limits` is the same category.

**Required llmadapter changes:**

- Add `Limits modeldb.Limits` field to `ModelResolutionCandidate`
- Add `modelLimitsFromCatalog` helper to populate it during resolution
- Add `ContextWindow int` field to `AutoRouteSummary`
- Populate `ContextWindow` from `resolution.Limits.ContextWindow` in
  `RouteSummary()`

~25 lines of production code in llmadapter. Fully backward compatible.

## 5. Design: Usage Data in Thread Event Log

### 5.1 Problem

Usage records (`usage.Record`) are tracked in-memory via `usage.Tracker` in
`agent.recordEvent`. They are never persisted to the thread event store. This
means:

- Usage data is lost when a session is resumed from disk.
- There is no audit trail of per-turn token consumption.
- Cost tracking across sessions requires external instrumentation.

### 5.2 Design

Persist each `usage.Record` as a thread event alongside the existing
in-memory tracker recording.

**Thread event kind:** `"harness.usage_recorded"`

**Payload:** The `usage.Record` struct serialized as JSON. It already has
complete JSON tags including `dims`, `usage`, `source`, `recorded_at`.

**Emit point:** In `agent.recordEvent`, immediately after the
`tracker.Record(...)` call, when a `runner.UsageEvent` is received:

```go
case runner.UsageEvent:
    record := usage.FromRunnerEvent(ev, opts)
    a.tracker.Record(record)
    // NEW: persist to thread event log
    if a.threadRuntime != nil && a.threadRuntime.Live() != nil {
        raw, _ := json.Marshal(record)
        _ = a.threadRuntime.Live().Append(ctx, thread.Event{
            Kind:    "harness.usage_recorded",
            Payload: raw,
            Source:  thread.EventSource{Type: "session", SessionID: a.sessionID},
        })
    }
```

**Resume replay:** On session resume, replay `harness.usage_recorded` events
to rebuild the tracker. This can be done in a new helper or as part of the
existing `ResumeThreadRuntime` flow.

### 5.3 Why this is low complexity

- `usage.Record` already has JSON tags — no new serialization.
- The thread event append path is well-established (used by skill events,
  context events, provider metadata events).
- The tracker's `Record` method is idempotent for replay.
- ~10 lines of emit code, ~15 lines of replay code.

### 5.4 What this enables

- **Session resume with usage history.** Resumed sessions show accurate
  cumulative token counts.
- **Audit trail.** Every turn's token consumption is recorded in the durable
  event log alongside conversation events.
- **Cross-session analytics.** External tools can read the JSONL thread store
  and aggregate usage across sessions.
- **Auto-compaction accuracy.** The auto-compaction threshold check can
  optionally use actual provider-reported input tokens (from usage events)
  instead of local estimates, though the local estimate is sufficient for v1.

## 6. Design: Compaction Floor

### 6.1 Problem

After compaction, `tree.Path(branch)` still walks from HEAD to root through
all replaced nodes. `ProjectItems` then scans the full path, builds an index
of all nodes, collects all `Replaces` lists, and filters. With N total
historical nodes and K compaction cycles, every projection is O(N) even though
the projected output is only the last summary + keep window.

This cost is paid on **every turn** (via `BuildRequestForProvider` →
`ProjectItems`), not just on resume.

### 6.2 Design

Add a **compaction floor** to the tree — a per-branch pointer that tells
`Path()` to stop walking at a given node instead of continuing to root.

```go
type Tree struct {
    mu       sync.RWMutex
    nodes    map[NodeID]Node
    branches map[BranchID]NodeID
    floors   map[BranchID]NodeID  // NEW
}
```

**`SetFloor(branch, nodeID)`** — sets the floor for a branch. After this,
`Path(branch)` stops at `nodeID` (inclusive) instead of walking to root.

**`Path(branch)`** — modified to break when it reaches the floor node:

```go
func (t *Tree) Path(branch BranchID) ([]Node, error) {
    // ...
    floor := t.floors[branch]
    var reversed []Node
    for id := head; id != ""; {
        node, ok := t.nodes[id]
        if !ok {
            return nil, fmt.Errorf("conversation: node %q not found", id)
        }
        reversed = append(reversed, node)
        if id == floor {
            break
        }
        id = node.Parent
    }
    // ... reverse and return
}
```

### 6.3 When to set the floor

In `History.CompactContext`, after appending the compaction event. The floor
is set to the compaction node itself — it becomes the oldest node in the
path, and its summary message replaces everything before it:

```go
func (h *History) CompactContext(ctx context.Context, summary string, replaces ...conversation.NodeID) (conversation.NodeID, error) {
    // ... existing validation and append ...
    id, err := h.AppendContext(ctx, conversation.CompactionEvent{...})
    if err != nil {
        return "", err
    }
    h.tree.SetFloor(h.branch, id)
    return id, err
}
```

### 6.4 Resume: restoring the floor

On resume (`ResumeHistoryFromThread`), the floor must be restored. Since
compaction events are replayed as regular nodes, the floor is recomputed by
finding the last compaction event on the branch:

```go
// During event replay in ResumeHistoryFromThread:
// After inserting all nodes, scan for the last compaction event and set floor.
```

This is derived state — no new event type needed. The last `CompactionEvent`
node on the branch determines the floor.

### 6.5 Effect on ProjectItems

With the floor in place, `Path()` returns only nodes from the compaction
event forward. `ProjectItems` no longer sees replaced nodes at all — the
`replaced` map and `compactionPlacement` logic still work correctly but
process far fewer nodes. For the common case (one active compaction), the
path contains only: `[compaction_summary, keep_msg_1, ..., keep_msg_N,
new_messages...]`.

### 6.6 What this does NOT do

- **Does not delete nodes.** All original nodes remain in `tree.nodes` and
  in the JSONL event store. The floor only affects `Path()` traversal.
- **Does not affect other branches.** Floors are per-branch. A fork from
  before the compaction point still sees the original nodes.
- **Does not require snapshots.** This is a simple pointer, not a
  serialized state snapshot. Resume cost is still O(all events) for the
  JSONL read, but the hot-path projection cost becomes O(active nodes).

## 7. Implementation Plan

### Phase 0: llmadapter — Model limits in resolution pipeline

**Package:** `llmadapter/adapterconfig`

1. Add `Limits modeldb.Limits` to `ModelResolutionCandidate`.
2. Add `modelLimitsFromCatalog` helper to resolve limits from the catalog
   during `ResolveModelCandidates`.
3. Add `ContextWindow int` to `AutoRouteSummary`.
4. Populate `ContextWindow` from `resolution.Limits.ContextWindow` in
   `RouteSummary()`.
5. Tests for limits flowing through resolution and summary.

### Phase 1: Compaction floor on conversation tree

**Package:** `conversation`

1. Add `floors` map to `Tree`.
2. Add `SetFloor(branch, nodeID)` method.
3. Modify `Path()` to stop at floor.
4. Tests for floor behavior.

### Phase 2: Usage data in thread event log

**Package:** `agent`

1. Add `EventUsageRecorded` constant.
2. In `recordEvent`, persist `usage.Record` as thread event after tracker
   recording.
3. Add replay helper for `harness.usage_recorded` events on session resume.
4. Tests for usage event persistence and replay.

### Phase 3: `agent.Instance.Compact` method

**Package:** `agent`  
**File:** `agent/agent.go`

Add a `Compact` method to `agent.Instance` that:

1. Gets the runtime engine.
2. Projects current items to estimate "before" tokens.
3. Generates a summary via a dedicated LLM call.
4. Selects node IDs to replace (all except keep window and existing
   compactions).
5. Calls `engine.Compact(ctx, summary, nodeIDs...)`.
6. Returns a `CompactionResult` with before/after token estimates.

```go
type CompactionResult struct {
    ReplacedCount    int
    TokensBefore     int
    TokensAfter      int
    CompactionNodeID conversation.NodeID
}

func (a *Instance) Compact(ctx context.Context) (CompactionResult, error)
```

The method also needs a variant that accepts a keep window override:

```go
func (a *Instance) CompactWithOptions(ctx context.Context, opts CompactOptions) (CompactionResult, error)

type CompactOptions struct {
    KeepWindow int    // 0 = use default (4)
    Summary    string // if non-empty, skip LLM summarization
}
```

### Phase 4: `/compact` slash command

**Package:** `app`  
**File:** `app/app.go` (in `builtins()`)

Register the command in `builtins()`. The handler:

1. Gets the default agent.
2. Calls `inst.Compact(ctx)`.
3. Formats the result as `command.Text(...)`.

### Phase 5: Auto-compaction with model-aware threshold

**Package:** `agent`  
**Files:** `agent/agent.go`, `agent/options.go`

1. Add `contextWindow int` field to `Instance`.
2. Populate `contextWindow` from `AutoRouteSummary.ContextWindow` in
   `resolveRouteIdentity`.
3. Also populate `contextproviders.ModelInfo.ContextWindow` (currently
   missing).
4. Add `AutoCompactionConfig` and `WithAutoCompaction` option.
5. Add `autoCompactionThreshold()` method: explicit override → 80% of
   context window → fallback 80K.
6. In `RunTurn`, after the engine turn completes successfully, call a private
   `maybeAutoCompact` method.
7. `maybeAutoCompact` estimates tokens, checks threshold, and calls `Compact`
   if exceeded.
8. Emit the auto-compaction thread event.

### Phase 6: Set floor after compaction + restore on resume

**Packages:** `runtime`, `conversation`

1. In `History.CompactContext`, call `tree.SetFloor(branch, id)` after
   appending the compaction event.
2. In `ResumeHistoryFromThread`, after replaying all events, scan for the
   last `CompactionEvent` node and set the floor.
3. Tests for floor being set after compaction and restored on resume.

### Phase 7: Tests

- `conversation/tree_test.go` — floor traversal tests.
- `agent/agent_test.go` — test `Compact` with a mock client.
- `agent/agent_test.go` — test usage event persistence.
- `agent/agent_test.go` — test auto-compaction trigger logic with
  model-aware threshold.
- `app/app_test.go` — test `/compact` command dispatch.
- `runtime/history_commit_test.go` or `runtime/thread_runtime_test.go` —
  test floor set after compaction and restored on resume.

## 8. Trade-offs and Decisions

| Decision | Choice | Alternative | Rationale |
|----------|--------|-------------|-----------|
| Summary generation | LLM call | Text extraction | Higher quality summaries preserve semantic context |
| Keep window default | 4 messages | Configurable from start | Simple default covers most cases; can add flag later |
| Auto-compaction trigger | Token threshold | Message count / turn count | Directly measures context pressure |
| Auto-compaction threshold | 80% of model context window | Fixed 80K | Model-aware; adapts to 128K, 200K, 1M windows |
| Context window source | modeldb via llmadapter resolution | Direct modeldb lookup from agentsdk | Clean layering; every consumer benefits |
| Auto-compaction timing | After turn completion | Before turn start | Conversation is stable; user isn't waiting |
| Failure behavior | Log and continue | Fail the turn | Compaction is optimization, not correctness |
| Command availability | Always (builtin) | Config-gated | Core session management, like /new and /session |
| Agent-callable | No | Yes | Agent should not self-compact mid-turn; auto handles between-turns |
| Auto-compaction default | Disabled | Enabled | Explicit opt-in avoids surprise API calls and behavior changes |
| Usage persistence | Thread event log | Separate store | Consistent with all other durable state; enables resume |
| Usage event kind | `harness.usage_recorded` | `conversation.usage` | Usage is harness-level metadata, not conversation content |
| Tree optimization | Compaction floor (checkpoint) | Full event-sourcing snapshot | Floor fixes the hot-path O(N); snapshots solve cold-path which isn't a real problem yet |
| Floor restore | Derived from last CompactionEvent | Persisted as separate event | Less event types; floor is derived state not primary state |
| Tree mutation | Append-only, filter at projection | Fork/re-parent at compaction | Append-only preserves node IDs, continuations, audit trail |

## 9. Open Questions

1. **Summary model:** Should the summarization call use the same model as the
   agent, or a cheaper/faster model? Using the same model is simpler but more
   expensive. A future enhancement could allow configuring a separate
   summarization model.

2. **Context fragments:** When compaction replaces conversation nodes, context
   fragments (system context) are unaffected — they live outside the
   conversation tree. The `ThreadRuntime.Compact` already handles re-rendering
   context with `PreferFull`. Should the summary include a note about active
   context state, or is the re-rendered context sufficient?

3. **Branch awareness:** Compaction currently operates on the active branch
   only. The replaced nodes remain in the tree (they're just excluded from
   projection). This is correct per the existing design — compaction is a
   projection-level operation, not a tree mutation.

4. **Multiple compactions:** If a conversation is compacted multiple times,
   each compaction replaces a window of messages (which may include previous
   compaction summaries). The `placeCompaction` logic in `ProjectItems` already
   handles this correctly — it skips nodes marked as replaced and inserts the
   compaction summary at the position of the earliest replaced node. With the
   compaction floor, repeated compactions are even simpler: the floor advances
   to the latest compaction node, and `Path()` only sees the latest summary
   forward.

5. **Usage replay deduplication:** When replaying `harness.usage_recorded`
   events on resume, the tracker accumulates records. If the same session is
   resumed multiple times, records should not be double-counted. The current
   `Tracker.Record` appends unconditionally — replay should use a
   `requestID`-based dedup or reset-before-replay strategy.

## 10. Dependencies

- **llmadapter** — requires a new release with `Limits` on
  `ModelResolutionCandidate` and `ContextWindow` on `AutoRouteSummary`.
- Uses existing `conversation.EstimateMessagesTokens` for token estimation.
- Uses existing `runtime.Engine.Compact` for the actual compaction operation.
- Uses existing `unified.Client` for summary generation (same client as agent).
- Uses existing `modeldb.Limits` type (already a direct dependency).

## 11. Files to Change

### llmadapter (prerequisite release)

```
adapterconfig/model_resolution.go  — Add Limits to ModelResolutionCandidate,
                                     add modelLimitsFromCatalog helper
adapterconfig/auto_summary.go      — Add ContextWindow to AutoRouteSummary,
                                     populate from resolution
adapterconfig/*_test.go            — Tests for limits flow
```

### agentsdk

```
conversation/tree.go    — Add floors map, SetFloor method, modify Path()
agent/agent.go          — Add contextWindow field, populate in resolveRouteIdentity,
                          add Compact, CompactWithOptions, maybeAutoCompact,
                          autoCompactionThreshold, usage event persistence,
                          AutoCompactionConfig, CompactionResult, ErrNothingToCompact
agent/options.go        — Add WithAutoCompaction option
runtime/history.go      — Set floor after CompactContext, restore floor on resume
app/app.go              — Add /compact to builtins()
conversation/tree_test.go — Floor tests
agent/agent_test.go     — Tests for Compact, auto-compaction, usage events
app/app_test.go         — Test /compact command
runtime/*_test.go       — Floor set/restore tests
```

## 12. Dependency Chain

Per AGENTS.md dependency update process:

1. Implement and release llmadapter with `Limits`/`ContextWindow` additions.
2. Update agentsdk to new llmadapter version.
3. Implement all agentsdk changes (floor, usage events, compact, auto-compact).
4. Run `go test ./...` in agentsdk.
5. Tag and release agentsdk.
6. Update consumers (miniagent) to new agentsdk + llmadapter.
7. `task install` in miniagent before smoke testing.

## 13. Key Code Locations (for implementation reference)

These are the exact files and line ranges an implementer needs to read:

| What | File | Lines | Why |
|------|------|-------|-----|
| Tree struct + Path() | `conversation/tree.go` | 16-176 | Add floors, modify Path |
| CompactionEvent type | `conversation/event.go` | 24-29 | Payload for compaction nodes |
| ProjectItems (projection) | `conversation/item.go` | 39-128 | Understand replacement logic |
| compactionMessage | `conversation/item.go` | 301-313 | Summary → user message |
| EstimateMessagesTokens | `conversation/projection_policy.go` | 40-66 | Token estimation |
| History.CompactContext | `runtime/history.go` | 185-202 | Where to set floor |
| History.appendPayloads | `runtime/history.go` | 312-375 | How nodes are appended |
| ResumeHistoryFromThread | `runtime/history.go` | 61-100 | Where to restore floor |
| Engine.Compact | `runtime/runtime.go` | 155-166 | Delegates to ThreadRuntime or History |
| ThreadRuntime.Compact | `runtime/thread_runtime.go` | 272-291 | Compacts + re-renders context |
| Instance.RunTurn | `agent/agent.go` | 409-432 | Where to add maybeAutoCompact |
| Instance.recordEvent | `agent/agent.go` | 1040-1057 | Where to add usage persistence |
| Instance.resolveRouteIdentity | `agent/agent.go` | ~714-723 | Where to store contextWindow |
| Instance.contextProviders | `agent/agent.go` | ~950-956 | Where to populate ModelInfo.ContextWindow |
| App.builtins | `app/app.go` | 664-749 | Where to add /compact |
| Instance options | `agent/options.go` | 1-257 | Where to add WithAutoCompaction |
| runnertest.Client | `runnertest/client.go` | 1-114 | Mock client for tests |
| Existing compaction test | `runtime/thread_runtime_test.go` | 460-530 | Pattern for compaction tests |
| Existing app test | `app/app_test.go` | 92-115 | Pattern for command tests |
