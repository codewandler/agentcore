# PLAN: LLMAdapter Use-Case Selection in Agentsdk

Status: implemented in agentsdk, awaiting review/release  
Created: 2026-04-26  
Last refined: 2026-04-26  
Context: agentsdk upgraded `github.com/codewandler/llmadapter` from
`v0.48.22` to `v1.0.0-rc.6`.

## Goal

Use llmadapter's compatibility and use-case selection APIs so agentsdk can
prefer, inspect, or require model/provider/API routes that are suitable for
agent workloads.

The first target is `agentic_coding`, because miniagent and agentsdk's default
terminal agent are coding-agent shaped: they need streaming, tools, tool
continuation, reasoning, prompt caching, structured output, usage, and cache
accounting.

## Non-Goals

- Do not build an agentsdk-specific model compatibility format.
- Do not make compatibility warnings silently change routing unless a policy
  explicitly asks for route selection.
- Do not depend on llmadapter's repository checkout existing at runtime.
- Do not add compatibility behavior to miniagent until the agentsdk API is
  implemented, tested, and released.

## Current Status

- [x] Upgrade agentsdk to `github.com/codewandler/llmadapter v1.0.0-rc.6`.
- [x] Run `go mod tidy`.
- [x] Add grouped terminal CLI profiles for branded consumers.
- [x] Rename model compatibility CLI flags to `--model-*`.
- [x] Verify current code with:
  - `go test ./...`
  - `go vet ./...`
  - `go run ./cmd/agentsdk discover --local .`
  - `go run ./cmd/agentsdk run .`
  - `git diff --check`
- [x] Implement use-case policy support.
- [ ] Release agentsdk after implementation review.

## LLMAdapter Features To Use

- `compatibility.UseCaseAgenticCoding`
- `compatibility.UseCaseSummarization`
- `compatibility.ParseUseCase`
- `adapterconfig.LoadCompatibilityEvidence`
- `adapterconfig.UseCaseSelectionOptions`
- `adapterconfig.AutoResult.SelectModelForUseCase`
- `adapterconfig.EvaluateCompatibilityCandidates`
- `adapterconfig.CompatibleCandidates`
- `adapterconfig.NewMuxClient`
- `adapterconfig.ResolveModelCandidates`

## Architecture Decisions

### Policy Ownership

Model compatibility policy belongs to the agent runtime setup, because that is
where the llmadapter client, source API, model alias, and route metadata come
together.

App manifests, Go app defaults, and CLI flags are configuration sources only.
They should feed policy into `agent.Instance`; they should not duplicate route
selection logic.

### Policy Type

Add an agentsdk-level policy type in `agent`:

```go
type ModelUseCase string

const (
    ModelUseCaseAgenticCoding ModelUseCase = "agentic_coding"
    ModelUseCaseSummarization ModelUseCase = "summarization"
)

type ModelPolicy struct {
    UseCase       ModelUseCase
    SourceAPI     adapt.ApiKind
    ApprovedOnly  bool
    AllowDegraded bool
    AllowUntested bool
    EvidencePath  string
}
```

Add `agent.WithModelPolicy(policy ModelPolicy)`.

Default behavior remains unchanged when the policy is zero-value. That means
plain agentsdk and existing branded Go apps keep today's route selection.

### Source API Selection

Agentsdk currently defaults `sourceAPI` to `openai.responses`. That is fine for
normal execution, but it is too narrow for use-case selection because approved
agentic routes may exist through `anthropic.messages`, `openai.responses`,
`openai.chat.completions`, or provider-specific routes.

Add an explicit source API surface:

```text
--source-api auto|openai.responses|openai.chat.completions|anthropic.messages
```

Rules:

1. No model policy and no `--source-api`: keep existing `openai.responses`
   default.
2. Model policy configured and no source API override: select across all source
   APIs by passing the zero `adapt.ApiKind` to llmadapter selection.
3. Explicit `--source-api`: restrict both compatibility selection and runtime
   routing to that source API.
4. After approved-only selection chooses a route, set the runtime source API to
   the selected route source API so the mux cannot drift to another source API.

### Evidence Loading

Do not use `adapterconfig.DefaultCompatibilityEvidencePath` as the agentsdk
default. That path points into the llmadapter repository and is not reliable for
installed agentsdk binaries.

Create an agentsdk evidence loader:

1. If `ModelPolicy.EvidencePath` is set, load that file with
   `adapterconfig.LoadCompatibilityEvidence`.
2. Otherwise load an embedded evidence snapshot bundled in agentsdk.
3. Keep the embedded artifact format byte-for-byte compatible with llmadapter
   evidence JSON.
4. Refresh the embedded artifact deliberately when upgrading llmadapter.

Recommended location:

```text
agent/modelpolicy/evidence/agentic_coding.json
agent/modelpolicy/evidence/summarization.json
```

The first implementation may embed only `agentic_coding`. If `summarization` has
no bundled evidence yet, requesting it without `--model-compat-evidence` should fail
with a clear error.

### Approved-Only Route Pinning

Approved-only must be real route selection, not a warning.

Flow:

1. Build auto mux config as today.
2. Load compatibility evidence.
3. Call `AutoResult.SelectModelForUseCase(model, sourceAPI, opts)`.
4. Build a pinned `adapterconfig.Config` that contains only the selected
   provider endpoint and a single exact route.
5. Rebuild the runtime client with `adapterconfig.NewMuxClient(pinnedConfig,
   adapterconfig.WithSourceAPI(selection.Resolution.SourceAPI),
   adapterconfig.WithFallback(false))`.
6. Store the selected compatibility details on `agent.Instance`.
7. Fail closed when no approved route exists.

The pinned route should use `selection.Resolution`:

- `SourceAPI`
- `Provider`
- `ProviderAPI`
- `PublicModel`
- `NativeModel`
- `ModelDBService`
- `Weight`

The route must be exact. Do not keep broad dynamic routes in approved-only mode,
because doing so reintroduces runtime ambiguity.

### Evaluation-Only Mode

When `UseCase` is set and `ApprovedOnly` is false:

1. Build auto mux as today.
2. Load evidence if available.
3. Evaluate candidates with `adapterconfig.EvaluateCompatibilityCandidates`.
4. Keep normal routing behavior.
5. Store compatibility diagnostics for verbose output.

If evidence is missing in evaluation-only mode, do not fail startup. Record a
diagnostic saying compatibility evidence was unavailable.

### Display

Extend `ParamsSummary` or add a route-policy summary method so verbose REPL and
run output can show:

```text
model: haiku  resolved_instance: claude  resolved_model: claude-haiku-4-5-20251001  source_api: anthropic.messages  provider_api: anthropic.messages  use_case: agentic_coding  compatibility: approved
```

For non-approved or diagnostic states:

```text
compatibility: failed  missing_required: reasoning, cache_accounting
compatibility: unavailable  reason: evidence not loaded
```

Keep normal non-verbose output unchanged.

## CLI Design

Add to `terminal/cli.NewCommand`:

```text
--source-api auto|openai.responses|openai.chat.completions|anthropic.messages
--model-use-case agentic_coding|summarization
--model-approved-only
--model-allow-degraded
--model-allow-untested
--model-compat-evidence <path>
```

Behavior:

- No flags: unchanged.
- `--model-use-case agentic_coding`: evaluation-only diagnostics.
- `--model-approved-only`: implies `--model-use-case agentic_coding` unless another use case
  is supplied.
- `--model-approved-only --model-allow-degraded`: accepts degraded evidence rows.
- `--model-approved-only --model-allow-untested`: accepts untested evidence rows.
- `--model-compat-evidence`: overrides bundled evidence.
- `--source-api auto`: passes zero `adapt.ApiKind` to selection/routing.

## Manifest Design

Extend `agentdir.AppManifest`:

```json
{
  "model_policy": {
    "use_case": "agentic_coding",
    "source_api": "auto",
    "approved_only": true,
    "allow_degraded": false,
    "allow_untested": false,
    "evidence_path": ".agentsdk/compatibility/agentic_coding.json"
  }
}
```

Precedence:

1. CLI flags
2. app manifest `model_policy`
3. branded Go app defaults
4. zero-value/unconfigured policy

## Execution Steps

### Phase 1: Policy Types and Evidence Loader

- [x] Add `agent.ModelPolicy` and `agent.WithModelPolicy`.
- [x] Add compatibility state storage on `agent.Instance`.
- [x] Add source API parsing helpers for `auto`, `openai.responses`,
  `openai.chat.completions`, and `anthropic.messages`.
- [x] Add embedded evidence loader with path override support.
- [x] Add tests for zero policy, option copying, source API parsing, and
  evidence loading.

### Phase 2: Runtime Selection and Pinning

- [x] Add helper to select approved route from `adapterconfig.AutoResult`.
- [x] Add helper to construct a pinned `adapterconfig.Config` from
  `adapterconfig.UseCaseModelSelection`.
- [x] Rebuild the mux client with the pinned config in approved-only mode.
- [x] Disable fallback for approved-only pinned clients.
- [x] Update route identity from the selected resolution.
- [x] Add tests for approved route pinning, missing evidence, no approved route,
  degraded allowed/blocked, and untested allowed/blocked.

### Phase 3: Evaluation Diagnostics

- [x] Evaluate candidates when `UseCase` is set without `ApprovedOnly`.
- [x] Store compatibility status and feature failures on `agent.Instance`.
- [x] Extend verbose params summary.
- [x] Add tests for approved, failed, and missing-evidence diagnostics.

### Phase 4: CLI Flags

- [x] Add `--source-api`.
- [x] Add `--model-use-case`.
- [x] Add `--model-approved-only`.
- [x] Add `--model-allow-degraded`.
- [x] Add `--model-allow-untested`.
- [x] Add `--model-compat-evidence`.
- [x] Thread flags through `terminal/cli.Config` into `agent.WithModelPolicy`
  and `agent.WithSourceAPI`.
- [x] Add CLI tests for flag parsing and precedence.

### Phase 5: Manifest Model Policy

- [x] Add `ModelPolicy` to `agentdir.AppManifest`.
- [x] Add policy to `agentdir.Resolution`.
- [x] Apply manifest policy in `terminal/cli.Run` unless CLI overrides it.
- [x] Add manifest tests for policy parsing, source API parsing, evidence path
  resolution, and CLI override.

### Phase 6: Model Inspection Command

- [x] Add `agentsdk models`.
- [x] Print model, source API, provider, provider API, native model, use-case
  status, capability source, and failing/degraded features.
- [x] Support `--model-use-case`, `--model-approved-only`, `--model-allow-degraded`,
  `--model-allow-untested`, `--model-compat-evidence`, and `--source-api`.
- [x] Keep first iteration text-only; add JSON output later if needed.

### Phase 7: Docs and Verification

- [x] Update README and docs with model policy, source API, bundled evidence,
  and approved-only behavior.
- [x] Update `docs/RESOURCES.md` if manifest policy semantics affect resource
  loading docs.
- [x] Run `go test ./...`.
- [x] Run `go vet ./...`.
- [x] Run `git diff --check`.
- [ ] Smoke:
  - [x] `go run ./cmd/agentsdk discover --local .`
  - [x] `go run ./cmd/agentsdk run .`
  - [x] `go run ./cmd/agentsdk run . --model-use-case agentic_coding -v`
  - [x] `go run ./cmd/agentsdk run . --model-approved-only -v`
  - [x] `go run ./cmd/agentsdk models --model-use-case agentic_coding`

### Phase 8: Release Agentsdk

- [ ] Review final diff.
- [ ] Commit agentsdk.
- [ ] Tag agentsdk.
- [ ] Push branch and tag.
- [ ] Cut GitHub release.

### Phase 9: Consumer Rollout

- [ ] Update miniagent to the released agentsdk version.
- [ ] Decide whether miniagent defaults to advisory `agentic_coding` diagnostics
  or strict approved-only. Recommendation: advisory by default first, expose
  strict approved-only as a flag.
- [ ] Rebuild installed miniagent with `task install`.
- [ ] Run miniagent tests and smoke both execution paths.

## Deferred

- JSON output for `agentsdk models`.
- Interactive model/policy selector.
- Persisted compatibility decisions in app manifests.
- Remote evidence update command.
- Per-agent policy in resource bundles beyond app manifest defaults.

## Remaining Review Points

- Confirm the exact embedded evidence file source before implementation. The
  preferred source is llmadapter's released `agentic_coding` compatibility
  artifact matching `v1.0.0-rc.6`.
- Confirm whether `summarization` should be exposed in the first CLI surface if
  we do not bundle summarization evidence yet. Recommendation: parse it in the
  API, but fail clearly unless evidence is supplied.
