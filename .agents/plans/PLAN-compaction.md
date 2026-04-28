# PLAN: Conversation Compaction — Implementation

**Status:** Ready (rev 3 — final)  
**Date:** 2026-04-28  
**Design:** [DESIGN-compaction.md](DESIGN-compaction.md)

## Overview

Eight implementation phases, each independently testable and committable:

0. llmadapter — Model limits in resolution pipeline (prerequisite release)
1. Compaction floor on conversation tree
2. Usage data in thread event log
3. `agent.Instance.Compact` — core compaction method with LLM summarization
4. `/compact` builtin slash command
5. Auto-compaction with model-aware threshold
6. Set floor after compaction + restore on resume
7. Tests for all of the above

---

## Phase 0: llmadapter — Model limits in resolution pipeline

**Repo:** `llmadapter`  
**Prerequisite:** Must be released before agentsdk phases 1–6.

### 0.1 Add `Limits` to `ModelResolutionCandidate`

**File:** `adapterconfig/model_resolution.go`

Add field to the struct:

```go
type ModelResolutionCandidate struct {
	// ... existing fields ...
	Capabilities         router.CapabilitySet
	CapabilitySource     string
	Limits               modeldb.Limits    // NEW
	ConsumerContinuation unified.ContinuationMode
	InternalContinuation unified.ContinuationMode
	Transport            unified.TransportKind
}
```

Populate it in `ResolveModelCandidates`, where the candidate is constructed:

```go
		out = append(out, ModelResolutionCandidate{
			// ... existing fields ...
			Capabilities:         endpoint.Capabilities,
			CapabilitySource:     routeCapabilitySource(provider, endpoint, route, catalog, modelDBEnabled),
			Limits:               modelLimitsFromCatalog(catalog, route, endpoint, modelDBEnabled),
			ConsumerContinuation: endpoint.ConsumerContinuation,
			// ...
		})
```

### 0.2 Add `modelLimitsFromCatalog` helper

**File:** `adapterconfig/model_resolution.go`

```go
// modelLimitsFromCatalog resolves the model limits from the modeldb catalog
// for the given route. Returns zero Limits when modeldb is not enabled or the
// model is not found.
func modelLimitsFromCatalog(catalog modeldb.Catalog, route RouteConfig, endpoint router.ProviderEndpoint, modelDBEnabled bool) modeldb.Limits {
	if !modelDBEnabled {
		return modeldb.Limits{}
	}
	wireModelID := pricingWireModel(route)
	serviceID := endpoint.Tags[TagModelDBServiceID]
	if wireModelID == "" || serviceID == "" {
		return modeldb.Limits{}
	}
	offering, ok := catalog.Offerings[modeldb.OfferingRef{ServiceID: serviceID, WireModelID: wireModelID}]
	if !ok {
		return modeldb.Limits{}
	}
	model, ok := catalog.Models[offering.ModelKey]
	if !ok {
		return modeldb.Limits{}
	}
	return model.Limits
}
```

### 0.3 Add `ContextWindow` to `AutoRouteSummary`

**File:** `adapterconfig/auto_summary.go`

```go
type AutoRouteSummary struct {
	SourceAPI       adapt.ApiKind `json:"source_api,omitempty"`
	Model           string        `json:"model,omitempty"`
	Provider        string        `json:"provider,omitempty"`
	ProviderAPI     adapt.ApiKind `json:"provider_api,omitempty"`
	NativeModel     string        `json:"native_model,omitempty"`
	EnabledProvider string        `json:"enabled_provider,omitempty"`
	EnabledReason   string        `json:"enabled_reason,omitempty"`
	ContextWindow   int           `json:"context_window,omitempty"` // NEW
}
```

Populate in `RouteSummary()`:

```go
func (r AutoResult) RouteSummary(sourceAPI adapt.ApiKind, model string) (AutoRouteSummary, bool) {
	// ... existing code ...
	resolution, err := ResolveModel(r.Config, model, sourceAPI)
	if err != nil {
		return AutoRouteSummary{}, false
	}
	summary := AutoRouteSummary{
		SourceAPI:     resolution.SourceAPI,
		Model:         resolution.PublicModel,
		Provider:      resolution.Provider,
		ProviderAPI:   resolution.ProviderAPI,
		NativeModel:   resolution.NativeModel,
		ContextWindow: resolution.Limits.ContextWindow, // NEW
	}
	// ... rest unchanged ...
}
```

### 0.4 Tests

**File:** `adapterconfig/model_resolution_test.go` (or existing test file)

```go
func TestResolveModelCandidateCarriesLimits(t *testing.T) {
	cfg := testConfigWithModelDB(t, 200_000)
	candidates, err := ResolveModelCandidates(cfg, "test-model", "")
	require.NoError(t, err)
	require.NotEmpty(t, candidates)
	require.Equal(t, 200_000, candidates[0].Limits.ContextWindow)
}

func TestRouteSummaryIncludesContextWindow(t *testing.T) {
	result := testAutoResultWithModelDB(t, 128_000)
	summary, ok := result.RouteSummary("", "test-model")
	require.True(t, ok)
	require.Equal(t, 128_000, summary.ContextWindow)
}

func TestResolveModelWithoutModelDBReturnsZeroLimits(t *testing.T) {
	cfg := testConfigWithoutModelDB(t)
	candidates, err := ResolveModelCandidates(cfg, "test-model", "")
	require.NoError(t, err)
	require.NotEmpty(t, candidates)
	require.Equal(t, 0, candidates[0].Limits.ContextWindow)
}
```

### 0.5 Release

Tag and push llmadapter. Then update agentsdk's `go.mod` to the new version.

---

## Phase 1: Compaction floor on conversation tree

### 1.1 Add `floors` map to `Tree` — `conversation/tree.go`

```go
type Tree struct {
	mu       sync.RWMutex
	nodes    map[NodeID]Node
	branches map[BranchID]NodeID
	floors   map[BranchID]NodeID // NEW
}

func NewTree() *Tree {
	return &Tree{
		nodes:    make(map[NodeID]Node),
		branches: map[BranchID]NodeID{MainBranch: ""},
		floors:   make(map[BranchID]NodeID), // NEW
	}
}
```

### 1.2 Add `SetFloor` and `Floor` methods — `conversation/tree.go`

```go
// SetFloor sets the compaction floor for a branch. Path() will stop walking
// at this node (inclusive) instead of continuing to root. An empty nodeID
// clears the floor.
func (t *Tree) SetFloor(branch BranchID, nodeID NodeID) {
	if branch == "" {
		branch = MainBranch
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if nodeID == "" {
		delete(t.floors, branch)
		return
	}
	t.floors[branch] = nodeID
}

// Floor returns the compaction floor for a branch, if set.
func (t *Tree) Floor(branch BranchID) (NodeID, bool) {
	if branch == "" {
		branch = MainBranch
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	id, ok := t.floors[branch]
	return id, ok && id != ""
}
```

### 1.3 Modify `Path` to stop at floor — `conversation/tree.go`

Replace the existing `Path` method:

```go
func (t *Tree) Path(branch BranchID) ([]Node, error) {
	if branch == "" {
		branch = MainBranch
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	head, ok := t.branches[branch]
	if !ok {
		return nil, fmt.Errorf("conversation: branch %q not found", branch)
	}
	floor := t.floors[branch] // "" means walk to root
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
	out := make([]Node, len(reversed))
	for i := range reversed {
		out[len(reversed)-1-i] = reversed[i]
	}
	return out, nil
}
```

### 1.4 Tests — `conversation/tree_test.go`

```go
func TestTreePathStopsAtFloor(t *testing.T) {
	tree := NewTree()
	a, _ := tree.Append(MainBranch, MessageEvent{Message: unified.Message{Role: "user", Content: []unified.ContentPart{unified.TextPart{Text: "a"}}}})
	b, _ := tree.Append(MainBranch, MessageEvent{Message: unified.Message{Role: "user", Content: []unified.ContentPart{unified.TextPart{Text: "b"}}}})
	c, _ := tree.Append(MainBranch, MessageEvent{Message: unified.Message{Role: "user", Content: []unified.ContentPart{unified.TextPart{Text: "c"}}}})

	// Without floor: full path.
	path, err := tree.Path(MainBranch)
	require.NoError(t, err)
	require.Len(t, path, 3)

	// Set floor to b: path starts at b.
	tree.SetFloor(MainBranch, b)
	path, err = tree.Path(MainBranch)
	require.NoError(t, err)
	require.Len(t, path, 2)
	require.Equal(t, b, path[0].ID)
	require.Equal(t, c, path[1].ID)

	// Clear floor: full path again.
	tree.SetFloor(MainBranch, "")
	path, err = tree.Path(MainBranch)
	require.NoError(t, err)
	require.Len(t, path, 3)
	_ = a // suppress unused
}

func TestTreeFloorDoesNotAffectOtherBranches(t *testing.T) {
	tree := NewTree()
	tree.Append(MainBranch, MessageEvent{Message: unified.Message{Role: "user", Content: []unified.ContentPart{unified.TextPart{Text: "a"}}}})
	b, _ := tree.Append(MainBranch, MessageEvent{Message: unified.Message{Role: "user", Content: []unified.ContentPart{unified.TextPart{Text: "b"}}}})
	tree.Append(MainBranch, MessageEvent{Message: unified.Message{Role: "user", Content: []unified.ContentPart{unified.TextPart{Text: "c"}}}})

	require.NoError(t, tree.Fork(MainBranch, "other"))
	tree.SetFloor(MainBranch, b)

	// Main branch: 2 nodes (b, c).
	mainPath, _ := tree.Path(MainBranch)
	require.Len(t, mainPath, 2)

	// Other branch: still 3 nodes (forked before floor was set).
	otherPath, _ := tree.Path("other")
	require.Len(t, otherPath, 3)
}
```

---

## Phase 2: Usage data in thread event log

### 2.1 Event constant — `agent/agent.go`

Add near the existing `ErrMaxStepsReached`:

```go
const EventUsageRecorded thread.EventKind = "harness.usage_recorded"
```

### 2.2 Persist usage in `recordEvent` — `agent/agent.go`

Replace the existing `recordEvent` method:

```go
func (a *Instance) recordEvent(turnID int, event runner.Event) {
	switch ev := event.(type) {
	case runner.RouteEvent:
		a.providerIdentity = ev.ProviderIdentity
		a.resolvedProvider = ev.ProviderIdentity.ProviderName
		a.resolvedModel = ev.ProviderIdentity.NativeModel
	case runner.UsageEvent:
		record := usage.FromRunnerEvent(ev, usage.RunnerEventOptions{
			TurnID:        strconv.Itoa(turnID),
			SessionID:     a.sessionID,
			FallbackModel: a.inference.Model,
			RouteState: usage.RouteState{
				Provider: a.resolvedProvider,
				Model:    a.resolvedModel,
			},
		})
		a.tracker.Record(record)
		a.persistUsageEvent(record)
	}
}

// persistUsageEvent appends a usage record to the thread event log so it
// survives session resume.
func (a *Instance) persistUsageEvent(record usage.Record) {
	if a.threadRuntime == nil || a.threadRuntime.Live() == nil {
		return
	}
	raw, err := json.Marshal(record)
	if err != nil {
		return
	}
	_ = a.threadRuntime.Live().Append(context.Background(), thread.Event{
		Kind:    EventUsageRecorded,
		Payload: raw,
		Source:  thread.EventSource{Type: "session", SessionID: a.sessionID},
	})
}
```

### 2.3 Replay on resume — `agent/agent.go`

```go
// replayUsageEvents rebuilds the usage tracker from persisted thread events.
func (a *Instance) replayUsageEvents(events []thread.Event) {
	if a.tracker == nil {
		return
	}
	for _, event := range events {
		if event.Kind != EventUsageRecorded {
			continue
		}
		var record usage.Record
		if err := json.Unmarshal(event.Payload, &record); err != nil {
			continue
		}
		a.tracker.Record(record)
	}
}
```

Call site: in the session resume path, after events are loaded for the branch.

---

## Phase 3: `agent.Instance.Compact` method

### 3.1 Types — `agent/agent.go`

```go
var ErrNothingToCompact = errors.New("agent: nothing to compact")

type CompactOptions struct {
	KeepWindow int
	Summary    string
}

type CompactionResult struct {
	ReplacedCount    int
	TokensBefore     int
	TokensAfter      int
	CompactionNodeID conversation.NodeID
}

const defaultKeepWindow = 4
```

### 3.2 Compact method — `agent/agent.go`

```go
func (a *Instance) Compact(ctx context.Context) (CompactionResult, error) {
	return a.CompactWithOptions(ctx, CompactOptions{})
}

func (a *Instance) CompactWithOptions(ctx context.Context, opts CompactOptions) (CompactionResult, error) {
	if a == nil || a.runtime == nil {
		return CompactionResult{}, fmt.Errorf("agent: runtime is not initialized")
	}
	if a.client == nil {
		return CompactionResult{}, fmt.Errorf("agent: client is not initialized")
	}

	keepWindow := opts.KeepWindow
	if keepWindow <= 0 {
		keepWindow = defaultKeepWindow
	}

	history := a.runtime.History()
	if history == nil {
		return CompactionResult{}, fmt.Errorf("agent: history is not initialized")
	}

	messagesBefore, err := history.Messages()
	if err != nil {
		return CompactionResult{}, fmt.Errorf("agent: compact: %w", err)
	}
	tokensBefore := conversation.EstimateMessagesTokens(messagesBefore, nil)

	replaceIDs, keepCount, err := a.selectCompactionNodes(history, keepWindow)
	if err != nil {
		return CompactionResult{}, err
	}
	if len(replaceIDs) == 0 {
		return CompactionResult{}, ErrNothingToCompact
	}

	summary := strings.TrimSpace(opts.Summary)
	if summary == "" {
		toSummarize := compactionSummarizeMessages(messagesBefore, keepCount)
		generated, err := a.generateCompactionSummary(ctx, toSummarize)
		if err != nil {
			return CompactionResult{}, fmt.Errorf("agent: compact summary: %w", err)
		}
		summary = generated
	}

	nodeID, err := a.runtime.Compact(ctx, summary, replaceIDs...)
	if err != nil {
		return CompactionResult{}, fmt.Errorf("agent: compact: %w", err)
	}

	messagesAfter, err := history.Messages()
	if err != nil {
		return CompactionResult{
			ReplacedCount:    len(replaceIDs),
			TokensBefore:     tokensBefore,
			CompactionNodeID: nodeID,
		}, nil
	}
	tokensAfter := conversation.EstimateMessagesTokens(messagesAfter, nil)

	return CompactionResult{
		ReplacedCount:    len(replaceIDs),
		TokensBefore:     tokensBefore,
		TokensAfter:      tokensAfter,
		CompactionNodeID: nodeID,
	}, nil
}
```

### 3.3 Node selection helper — `agent/agent.go`

```go
func (a *Instance) selectCompactionNodes(history *agentruntime.History, keepWindow int) ([]conversation.NodeID, int, error) {
	tree := history.Tree()
	if tree == nil {
		return nil, 0, fmt.Errorf("agent: conversation tree is nil")
	}
	path, err := tree.Path(history.Branch())
	if err != nil {
		return nil, 0, fmt.Errorf("agent: compact: %w", err)
	}
	if len(path) <= keepWindow {
		return nil, 0, nil
	}

	cutoff := len(path) - keepWindow
	var replaceIDs []conversation.NodeID
	for _, node := range path[:cutoff] {
		if _, ok := node.Payload.(conversation.CompactionEvent); ok {
			continue
		}
		if _, ok := node.Payload.(*conversation.CompactionEvent); ok {
			continue
		}
		replaceIDs = append(replaceIDs, node.ID)
	}
	return replaceIDs, keepWindow, nil
}

func compactionSummarizeMessages(all []unified.Message, keepCount int) []unified.Message {
	if keepCount >= len(all) {
		return nil
	}
	return all[:len(all)-keepCount]
}
```

### 3.4 LLM summary generation — `agent/agent.go`

```go
const compactionSystemPrompt = `Summarize the following conversation concisely. Preserve:
- Key decisions and conclusions
- File paths, function names, and code identifiers mentioned
- Current task state and any pending work
- Important constraints or requirements established

Output only the summary. No preamble, no markdown headers.`

func (a *Instance) generateCompactionSummary(ctx context.Context, messages []unified.Message) (string, error) {
	if len(messages) == 0 {
		return "", fmt.Errorf("no messages to summarize")
	}

	var transcript strings.Builder
	for _, msg := range messages {
		fmt.Fprintf(&transcript, "[%s]", msg.Role)
		if msg.Name != "" {
			fmt.Fprintf(&transcript, " (%s)", msg.Name)
		}
		transcript.WriteString(": ")
		for _, part := range msg.Content {
			switch p := part.(type) {
			case unified.TextPart:
				transcript.WriteString(p.Text)
			case *unified.TextPart:
				if p != nil {
					transcript.WriteString(p.Text)
				}
			}
		}
		transcript.WriteByte('\n')
	}

	maxTokens := 1024
	req := unified.Request{
		Model:           a.inference.Model,
		MaxOutputTokens: &maxTokens,
		Instructions: []unified.Instruction{{
			Kind:    unified.InstructionSystem,
			Content: []unified.ContentPart{unified.TextPart{Text: compactionSystemPrompt}},
		}},
		Messages: []unified.Message{{
			Role:    unified.RoleUser,
			Content: []unified.ContentPart{unified.TextPart{Text: transcript.String()}},
		}},
		Stream: false,
	}

	events, err := a.client.Request(ctx, req)
	if err != nil {
		return "", err
	}

	var text strings.Builder
	for event := range events {
		switch ev := event.(type) {
		case unified.TextDeltaEvent:
			text.WriteString(ev.Text)
		case unified.ErrorEvent:
			if ev.Err != nil {
				return "", ev.Err
			}
		}
	}

	result := strings.TrimSpace(text.String())
	if result == "" {
		return "", fmt.Errorf("empty summary from model")
	}
	return result, nil
}
```

### 3.5 Token estimation helper — `agent/agent.go`

```go
func (a *Instance) estimateProjectedTokens() (int, error) {
	if a == nil || a.runtime == nil {
		return 0, fmt.Errorf("agent: runtime is not initialized")
	}
	history := a.runtime.History()
	if history == nil {
		return 0, fmt.Errorf("agent: history is not initialized")
	}
	messages, err := history.Messages()
	if err != nil {
		return 0, err
	}
	return conversation.EstimateMessagesTokens(messages, nil), nil
}
```

---

## Phase 4: `/compact` builtin slash command

### 4.1 Register in `app/app.go` — `builtins()`

Add to the `builtins()` slice, after the `/context` command:

```go
command.New(command.Spec{Name: "compact", Description: "Summarize and compact conversation history"}, func(ctx context.Context, _ command.Params) (command.Result, error) {
	inst, ok := a.DefaultAgent()
	if !ok {
		return command.Text("compact: no current agent"), nil
	}
	result, err := inst.Compact(ctx)
	if err != nil {
		if errors.Is(err, agent.ErrNothingToCompact) {
			return command.Text("compact: conversation too short to compact"), nil
		}
		return command.Text(fmt.Sprintf("compact: %v", err)), nil
	}
	saved := result.TokensBefore - result.TokensAfter
	return command.Text(fmt.Sprintf(
		"Compacted: replaced %d messages with summary\nEstimated tokens: before=%d after=%d (saved ~%d)",
		result.ReplacedCount, result.TokensBefore, result.TokensAfter, saved,
	)), nil
}),
```

---

## Phase 5: Auto-compaction with model-aware threshold

### 5.1 Instance fields — `agent/agent.go`

Add to the `Instance` struct:

```go
contextWindow   int
autoCompaction  AutoCompactionConfig
```

### 5.2 Config type — `agent/agent.go`

```go
type AutoCompactionConfig struct {
	Enabled        bool
	TokenThreshold int
	KeepWindow     int
}

const defaultAutoCompactionThreshold = 80_000
```

### 5.3 Option — `agent/options.go`

```go
func WithAutoCompaction(config AutoCompactionConfig) Option {
	return func(a *Instance) { a.autoCompaction = config }
}
```

### 5.4 Populate context window in `resolveRouteIdentity` — `agent/agent.go`

After `a.resolvedModel = summary.NativeModel` (around line 723), add:

```go
	a.contextWindow = summary.ContextWindow
```

### 5.5 Populate `ModelInfo.ContextWindow` — `agent/agent.go`

In `contextProviders()` (around line 952):

```go
	addIfNotOverridden(contextproviders.Model(contextproviders.ModelInfo{
		Name:          a.resolvedModel,
		Provider:      a.resolvedProvider,
		ContextWindow: a.contextWindow, // NEW
		Effort:        string(a.inference.Effort),
	}))
```

### 5.6 Threshold method — `agent/agent.go`

```go
func (a *Instance) autoCompactionThreshold() int {
	if a.autoCompaction.TokenThreshold > 0 {
		return a.autoCompaction.TokenThreshold
	}
	if a.contextWindow > 0 {
		return int(float64(a.contextWindow) * 0.8)
	}
	return defaultAutoCompactionThreshold
}
```

### 5.7 Hook in RunTurn — `agent/agent.go`

Add `a.maybeAutoCompact(ctx)` after the successful return from `runtime.RunTurn`,
before `return nil`.

### 5.8 Auto-compaction logic — `agent/agent.go`

```go
func (a *Instance) maybeAutoCompact(ctx context.Context) {
	if !a.autoCompaction.Enabled {
		return
	}
	threshold := a.autoCompactionThreshold()

	tokens, err := a.estimateProjectedTokens()
	if err != nil {
		return
	}
	if tokens < threshold {
		return
	}

	keepWindow := a.autoCompaction.KeepWindow
	if keepWindow <= 0 {
		keepWindow = defaultKeepWindow
	}

	result, err := a.CompactWithOptions(ctx, CompactOptions{KeepWindow: keepWindow})
	if err != nil {
		if a.verbose {
			fmt.Fprintf(a.Out(), "[auto-compact failed: %v]\n", err)
		}
		return
	}

	saved := result.TokensBefore - result.TokensAfter
	fmt.Fprintf(a.Out(), "[auto-compacted: replaced %d messages, ~%d tokens saved]\n",
		result.ReplacedCount, saved)

	a.emitAutoCompactionEvent(result, tokens, threshold)
}

func (a *Instance) emitAutoCompactionEvent(result CompactionResult, estimatedTokens, threshold int) {
	payload := map[string]any{
		"trigger":          "token_threshold",
		"estimated_tokens": estimatedTokens,
		"threshold":        threshold,
		"context_window":   a.contextWindow,
		"replaced_count":   result.ReplacedCount,
		"tokens_before":    result.TokensBefore,
		"tokens_after":     result.TokensAfter,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return
	}
	event := thread.Event{
		Kind:    "conversation.auto_compaction",
		Payload: raw,
	}
	if a.threadRuntime != nil && a.threadRuntime.Live() != nil {
		_ = a.threadRuntime.Live().Append(context.Background(), event)
	} else if a.history != nil {
		_ = a.history.AppendThreadEvents(context.Background(), event)
	}
}
```

---

## Phase 6: Set floor after compaction + restore on resume

### 6.1 Set floor in `History.CompactContext` — `runtime/history.go`

After the `AppendContext` call (line ~198), add:

```go
func (h *History) CompactContext(ctx context.Context, summary string, replaces ...conversation.NodeID) (conversation.NodeID, error) {
	// ... existing validation ...
	id, err := h.AppendContext(ctx, conversation.CompactionEvent{
		Summary:  summary,
		Replaces: append([]conversation.NodeID(nil), replaces...),
	})
	if err != nil {
		return "", err
	}
	h.tree.SetFloor(h.branch, id) // NEW
	return id, nil
}
```

### 6.2 Restore floor on resume — `runtime/history.go`

In `ResumeHistoryFromThread`, after the event replay loop (after line ~95),
scan for the last compaction node and set the floor:

```go
	// After all nodes are inserted, find the last compaction event and set floor.
	path, err := h.tree.Path(h.branch)
	if err == nil {
		for i := len(path) - 1; i >= 0; i-- {
			switch path[i].Payload.(type) {
			case conversation.CompactionEvent, *conversation.CompactionEvent:
				h.tree.SetFloor(h.branch, path[i].ID)
				goto floorSet
			}
		}
	}
floorSet:
```

Note: This scan is O(path length) but runs once on resume, not on every turn.
An alternative is to track the last compaction node ID during the event replay
loop itself (avoiding the extra Path call):

```go
	var lastCompactionNodeID conversation.NodeID
	for _, event := range events {
		// ... existing replay code ...
		if event.Kind == eventConversationCompaction {
			lastCompactionNodeID = conversation.NodeID(event.NodeID)
		}
	}
	if lastCompactionNodeID != "" {
		h.tree.SetFloor(h.branch, lastCompactionNodeID)
	}
```

This second approach is cleaner — it piggybacks on the existing replay loop.

---

## Phase 7: Tests

### 7.1 `conversation/tree_test.go` — Floor tests

(See Phase 1.4 above for full test code.)

### 7.2 `agent/agent_test.go` — Compact method

```go
func TestAgentCompactReplacesOlderMessages(t *testing.T) {
	client := runnertest.NewClient(
		runnertest.TextStream("resp1"),
		runnertest.TextStream("resp2"),
		runnertest.TextStream("resp3"),
		runnertest.TextStream("Summary of old conversation."),
	)
	a, err := New(
		WithClient(client),
		WithWorkspace(t.TempDir()),
		WithSystem("system"),
		WithInferenceOptions(InferenceOptions{Model: "test/model", MaxTokens: 1000}),
	)
	require.NoError(t, err)

	ctx := context.Background()
	require.NoError(t, a.RunTurn(ctx, 1, "old message 1"))
	require.NoError(t, a.RunTurn(ctx, 2, "old message 2"))
	require.NoError(t, a.RunTurn(ctx, 3, "recent message"))

	result, err := a.CompactWithOptions(ctx, CompactOptions{KeepWindow: 4})
	require.NoError(t, err)
	require.Equal(t, 2, result.ReplacedCount)
	require.Greater(t, result.TokensBefore, result.TokensAfter)
	require.NotEmpty(t, result.CompactionNodeID)

	require.Len(t, client.Requests(), 4)
	summaryReq := client.RequestAt(3)
	require.Len(t, summaryReq.Messages, 1)
	require.Equal(t, unified.RoleUser, summaryReq.Messages[0].Role)
}

func TestAgentCompactWithProvidedSummarySkipsLLMCall(t *testing.T) {
	client := runnertest.NewClient(
		runnertest.TextStream("resp1"),
		runnertest.TextStream("resp2"),
		runnertest.TextStream("resp3"),
	)
	a, err := New(
		WithClient(client),
		WithWorkspace(t.TempDir()),
		WithSystem("system"),
		WithInferenceOptions(InferenceOptions{Model: "test/model", MaxTokens: 1000}),
	)
	require.NoError(t, err)

	ctx := context.Background()
	require.NoError(t, a.RunTurn(ctx, 1, "old"))
	require.NoError(t, a.RunTurn(ctx, 2, "old2"))
	require.NoError(t, a.RunTurn(ctx, 3, "recent"))

	result, err := a.CompactWithOptions(ctx, CompactOptions{
		KeepWindow: 4,
		Summary:    "Manual summary.",
	})
	require.NoError(t, err)
	require.Equal(t, 2, result.ReplacedCount)
	require.Len(t, client.Requests(), 3)
}

func TestAgentCompactTooShortReturnsError(t *testing.T) {
	client := runnertest.NewClient(runnertest.TextStream("resp"))
	a, err := New(
		WithClient(client),
		WithWorkspace(t.TempDir()),
		WithInferenceOptions(InferenceOptions{Model: "test/model", MaxTokens: 1000}),
	)
	require.NoError(t, err)

	require.NoError(t, a.RunTurn(context.Background(), 1, "hello"))

	_, err = a.Compact(context.Background())
	require.ErrorIs(t, err, ErrNothingToCompact)
}
```

### 7.3 `agent/agent_test.go` — Auto-compaction

```go
func TestAgentAutoCompactionTriggersAboveThreshold(t *testing.T) {
	largeResponse := strings.Repeat("x", 400_000)
	client := runnertest.NewClient(
		runnertest.TextStream("resp1"),
		runnertest.TextStream(largeResponse),
		runnertest.TextStream("Summary of conversation."),
		runnertest.TextStream("resp3"),
	)
	var buf bytes.Buffer
	a, err := New(
		WithClient(client),
		WithWorkspace(t.TempDir()),
		WithOutput(&buf),
		WithInferenceOptions(InferenceOptions{Model: "test/model", MaxTokens: 1000}),
		WithAutoCompaction(AutoCompactionConfig{
			Enabled:        true,
			TokenThreshold: 1000,
			KeepWindow:     2,
		}),
	)
	require.NoError(t, err)

	ctx := context.Background()
	require.NoError(t, a.RunTurn(ctx, 1, "hello"))
	require.NoError(t, a.RunTurn(ctx, 2, "generate large"))
	require.Contains(t, buf.String(), "auto-compacted")
}

func TestAgentAutoCompactionDisabledByDefault(t *testing.T) {
	largeResponse := strings.Repeat("x", 400_000)
	client := runnertest.NewClient(runnertest.TextStream(largeResponse))
	var buf bytes.Buffer
	a, err := New(
		WithClient(client),
		WithWorkspace(t.TempDir()),
		WithOutput(&buf),
		WithInferenceOptions(InferenceOptions{Model: "test/model", MaxTokens: 1000}),
	)
	require.NoError(t, err)

	require.NoError(t, a.RunTurn(context.Background(), 1, "hello"))
	require.NotContains(t, buf.String(), "auto-compacted")
}

func TestAgentAutoCompactionThresholdUsesContextWindow(t *testing.T) {
	a := &Instance{
		contextWindow:  200_000,
		autoCompaction: AutoCompactionConfig{Enabled: true},
	}
	require.Equal(t, 160_000, a.autoCompactionThreshold())
}

func TestAgentAutoCompactionThresholdExplicitOverride(t *testing.T) {
	a := &Instance{
		contextWindow:  200_000,
		autoCompaction: AutoCompactionConfig{Enabled: true, TokenThreshold: 50_000},
	}
	require.Equal(t, 50_000, a.autoCompactionThreshold())
}

func TestAgentAutoCompactionThresholdFallback(t *testing.T) {
	a := &Instance{
		autoCompaction: AutoCompactionConfig{Enabled: true},
	}
	require.Equal(t, defaultAutoCompactionThreshold, a.autoCompactionThreshold())
}
```

### 7.4 `app/app_test.go` — `/compact` command

```go
func TestAppCompactCommandReturnsResult(t *testing.T) {
	client := runnertest.NewClient(
		runnertest.TextStream("resp1"),
		runnertest.TextStream("resp2"),
		runnertest.TextStream("resp3"),
		runnertest.TextStream("Summary."),
	)
	app, err := New(WithAgentSpec(agent.Spec{
		Name:      "coder",
		System:    "You code.",
		Inference: agent.InferenceOptions{Model: "test/model", MaxTokens: 1000},
	}), WithOutput(&bytes.Buffer{}))
	require.NoError(t, err)
	_, err = app.InstantiateAgent("coder",
		agent.WithClient(client),
		agent.WithWorkspace(t.TempDir()),
	)
	require.NoError(t, err)

	ctx := context.Background()
	_, err = app.Send(ctx, "old1")
	require.NoError(t, err)
	_, err = app.Send(ctx, "old2")
	require.NoError(t, err)
	_, err = app.Send(ctx, "recent")
	require.NoError(t, err)

	result, err := app.Send(ctx, "/compact")
	require.NoError(t, err)
	require.Equal(t, command.ResultText, result.Kind)
	require.Contains(t, result.Text, "Compacted")
	require.Contains(t, result.Text, "replaced")
}

func TestAppCompactCommandTooShort(t *testing.T) {
	client := runnertest.NewClient(runnertest.TextStream("resp"))
	app, err := New(WithAgentSpec(agent.Spec{
		Name:      "coder",
		System:    "You code.",
		Inference: agent.InferenceOptions{Model: "test/model", MaxTokens: 1000},
	}), WithOutput(&bytes.Buffer{}))
	require.NoError(t, err)
	_, err = app.InstantiateAgent("coder",
		agent.WithClient(client),
		agent.WithWorkspace(t.TempDir()),
	)
	require.NoError(t, err)

	ctx := context.Background()
	_, err = app.Send(ctx, "hello")
	require.NoError(t, err)

	result, err := app.Send(ctx, "/compact")
	require.NoError(t, err)
	require.Equal(t, command.ResultText, result.Kind)
	require.Contains(t, result.Text, "too short")
}
```

### 7.5 `runtime/*_test.go` — Floor set/restore after compaction

```go
func TestCompactContextSetsFloorOnTree(t *testing.T) {
	history := NewHistory(WithHistorySessionID("test"))
	_, _ = history.AddUser("old")
	_, _ = history.AddUser("keep")

	old := history.Tree().Path(history.Branch())
	// ... get first node ID ...

	compactionID, err := history.CompactContext(context.Background(), "summary", oldNodeID)
	require.NoError(t, err)

	floor, ok := history.Tree().Floor(history.Branch())
	require.True(t, ok)
	require.Equal(t, compactionID, floor)

	// Path should now start at compaction node.
	path, err := history.Tree().Path(history.Branch())
	require.NoError(t, err)
	require.Equal(t, compactionID, path[0].ID)
}
```

---

## File Change Summary

| File | Changes |
|------|---------|
| **llmadapter** (prerequisite release) | |
| `adapterconfig/model_resolution.go` | `Limits` field on `ModelResolutionCandidate`, `modelLimitsFromCatalog` helper |
| `adapterconfig/auto_summary.go` | `ContextWindow` field on `AutoRouteSummary`, populate from resolution |
| `adapterconfig/*_test.go` | Tests for limits flow |
| **agentsdk** | |
| `conversation/tree.go` | `floors` map, `SetFloor`, `Floor` methods, modify `Path` to stop at floor, update `NewTree` |
| `conversation/tree_test.go` | Floor traversal tests |
| `agent/agent.go` | `EventUsageRecorded`, `persistUsageEvent`, `replayUsageEvents`, `contextWindow` + `autoCompaction` fields, `ErrNothingToCompact`, `CompactOptions`, `CompactionResult`, `AutoCompactionConfig`, `defaultKeepWindow`, `defaultAutoCompactionThreshold`, `Compact`, `CompactWithOptions`, `selectCompactionNodes`, `compactionSummarizeMessages`, `generateCompactionSummary`, `estimateProjectedTokens`, `autoCompactionThreshold`, `maybeAutoCompact`, `emitAutoCompactionEvent`, modify `RunTurn`, modify `recordEvent`, modify `resolveRouteIdentity`, modify `contextProviders` |
| `agent/options.go` | `WithAutoCompaction` option |
| `runtime/history.go` | Set floor after `CompactContext`, restore floor in `ResumeHistoryFromThread` |
| `app/app.go` | `/compact` in `builtins()` |
| `agent/agent_test.go` | 8 test functions |
| `app/app_test.go` | 2 test functions |
| `runtime/*_test.go` | Floor set/restore tests |

## Implementation Order

```
0. llmadapter: Limits + ContextWindow            (prerequisite release)
   └─ tag, push, update agentsdk go.mod

1. conversation/tree.go: floors + SetFloor + Path  (no behavior change — floor unset by default)
2. agent/agent.go: usage event persistence         (no behavior change to turns)
3. agent/agent.go: types + Compact + helpers       (no behavior change to turns)
4. agent/options.go: WithAutoCompaction             (no behavior change)
5. agent/agent.go: contextWindow + resolveRouteIdentity + ModelInfo
                   + autoCompactionThreshold       (no behavior change)
6. runtime/history.go: SetFloor after CompactContext
                       + restore on resume         (activates floor for compacted sessions)
7. agent/agent.go: modify RunTurn + maybeAutoCompact
                                                   (behavior change, gated by Enabled=false)
8. app/app.go: /compact builtin                    (new command)
9. tests: conversation, agent, app, runtime
10. go test ./conversation/... ./agent/... ./app/... ./runtime/...
11. go test ./...
```

Each step can be committed independently. Steps 1–5 are pure additions with no
behavior change. Step 6 activates the floor but only for sessions that have
compaction events. Step 7 modifies `RunTurn` but is gated behind
`AutoCompactionConfig.Enabled` (default false). Step 8 adds the user-facing
command.
