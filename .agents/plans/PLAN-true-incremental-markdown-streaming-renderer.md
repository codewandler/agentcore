# PLAN: True Incremental Markdown Streaming Renderer

## Status

Planned. This is **Option D** from the markdown streaming investigation: use or build a true incremental Markdown parser/renderer instead of relying on block buffering plus heuristic inline detection.

## Problem statement

The current terminal streaming path stabilizes top-level Markdown blocks with `markdown.Buffer` and renders those blocks through `terminal/ui`'s goldmark-backed renderer. This is much better than hand-rendering inline Markdown in the stream coordinator, but it is still not a complete streaming Markdown implementation.

Current limitations:

- `terminal/ui/step.go` still decides whether incoming text is Markdown-looking with heuristics.
- `markdown.Buffer` is conservative but not a true incremental CommonMark parser.
- Inline Markdown correctness depends on whether the stream coordinator buffers the right chunk boundaries.
- Complete CommonMark compatibility is not proven for arbitrary chunking.
- The UX trade-off is awkward: buffering improves correctness but can delay visible output.

Goal:

Build a streaming Markdown system where streamed output is correct by construction for supported Markdown features, with no duplicate inline parsers and no ad-hoc chunk-boundary heuristics in `StepDisplay`.

## Constraints and assumptions

Assumptions:

- Primary target is terminal output for LLM responses.
- Go-only implementation is preferred unless an external dependency is clearly superior.
- The existing public `markdown.Buffer` API should remain source-compatible for at least one release if possible.
- `terminal/ui` should remain a consumer of reusable markdown streaming primitives, not the owner of parser logic.
- Full CommonMark + GFM parity is desirable, but the first implementation may ship a documented subset if tests make the subset explicit.

Constraints:

- Must not introduce new `flai` references.
- Must keep existing full test suite passing: `go test ./...`.
- Must preserve good code-fence streaming UX; code blocks should not wait for the closing fence before syntax-highlighted content appears if avoidable.
- Must avoid O(n²) behavior on long responses.
- Must be observable/testable with deterministic snapshot/equivalence tests.

## Component boundaries

Target module/layer split:

```text
runner events
  |
  v
terminal/ui.StepDisplay
  - owns turn display state only
  - does not parse Markdown syntax
  - delegates streamed assistant text to streammarkdown.Renderer
  |
  v
markdown/stream or terminal/streammarkdown
  - owns incremental parser state
  - emits stable render operations or terminal spans
  - owns chunk-boundary correctness
  |
  +--> markdown parser core
  |      - block state machine
  |      - inline delimiter state
  |      - source position tracking
  |
  +--> renderer adapter
         - terminal ANSI renderer
         - optional plain/debug renderer for tests
```

Recommended package placement:

- `markdown/stream`: reusable parser/event API with no terminal styling.
- `terminal/ui`: terminal-specific renderer over `markdown/stream` events.

Alternative:

- `terminal/streammarkdown`: combine parser and terminal rendering.

Recommendation: start with `markdown/stream` if we are willing to invest in clean contracts; otherwise start under `terminal/ui/internal/streammarkdown` to avoid prematurely freezing an API.

## Data flow

Current flow:

```text
TextDelta -> StepDisplay.writeTextChunk -> heuristics -> markdown.Buffer -> terminal renderer
                                  \-> fast/plain path
```

Target flow:

```text
TextDelta
  -> StepDisplay.WriteText
  -> streammarkdown.Writer.Write(chunk)
  -> incremental parser updates state
  -> renderer receives append-only events
  -> terminal output is written incrementally
  -> End() finalizes open constructs
```

The stream parser should expose one of these contracts:

### Contract A: render events

```go
type Event interface { eventKind() }

type TextEvent struct {
    Text string
    Style InlineStyle
}

type BlockStartEvent struct { Kind BlockKind }
type BlockEndEvent struct { Kind BlockKind }
type CodeLineEvent struct {
    Language string
    Text string
}

type Parser struct { ... }
func (p *Parser) Write(s string) ([]Event, error)
func (p *Parser) Flush() ([]Event, error)
```

Pros:

- Reusable outside terminals.
- Testable without ANSI snapshots.
- StepDisplay stays thin.

Cons:

- Harder to design completely.
- Inline/event model can become a second AST.

### Contract B: append-only rendered spans

```go
type Span struct {
    Text string
    Style SpanStyle
}

type Renderer struct { ... }
func (r *Renderer) Write(s string) ([]Span, error)
func (r *Renderer) Flush() ([]Span, error)
```

Pros:

- Smaller and easier to ship.
- Directly solves terminal UX.

Cons:

- Less reusable.
- Parser and renderer can become coupled.

Recommendation: begin with Contract B internally, but structure code so parser state and rendering are separable. Promote to Contract A only after the model survives tests.

## Parser architecture

CommonMark parsing is two-phase for complete documents:

1. Block parsing line-by-line.
2. Inline parsing inside completed paragraphs/headings.

For streaming, use a hybrid model:

### Block state machine

Track open block contexts:

- paragraph
- ATX heading
- fenced code block
- indented code block
- list/list item
- blockquote
- table candidate if GFM enabled
- HTML block candidate if supported

Rules:

- Process complete lines immediately.
- Keep incomplete trailing line buffered.
- Emit finalized leaf blocks when the parser knows subsequent input cannot change their block type.
- For fenced code blocks, emit code content lines incrementally after the opening fence is confirmed.

### Inline parser state

For paragraph text, choose one of two strategies:

#### Strategy 1: commit paragraphs only at block boundary

Buffer paragraph text until a blank line, block transition, or flush. Then parse inlines with goldmark or a custom inline parser.

Pros:

- Much easier to make CommonMark-compatible.
- Goldmark remains the source of truth for inlines.
- Good first milestone.

Cons:

- Inline-styled prose does not stream character-by-character.
- Long paragraphs render late unless force-split policies are used.

#### Strategy 2: true incremental inline delimiter stack

Implement CommonMark-style delimiter stack for `*`, `_`, links, images, code spans, autolinks, and raw HTML. Emit append-only styled spans when delimiters are resolved.

Pros:

- Best UX.
- Closest to true streaming Markdown.

Cons:

- Complex and easy to get wrong.
- Requires extensive CommonMark conformance tests.

Recommendation: ship Strategy 1 first as a correctness baseline, then evaluate Strategy 2 only if UX latency is unacceptable.

## Rendering architecture

Terminal rendering should be a pure consumer of parser output.

Renderer responsibilities:

- ANSI styling for emphasis, strong, code spans, links, headings, lists, tables, and blockquotes.
- Syntax highlighting for fenced code content.
- Controlled block spacing.
- No Markdown syntax decisions.

Renderer non-responsibilities:

- Detecting Markdown delimiters.
- Deciding whether `*` is multiplication or emphasis.
- Managing incomplete stream tails.

## Phased implementation plan

### Phase 0: Characterize current behavior

- Add a streaming equivalence test harness before major rewrite.
- Compare current streaming output to full render output for selected cases.
- Document failures as expected gaps.

Files:

- `terminal/ui/streaming_markdown_test.go` or `terminal/ui/ui_test.go`
- possibly `markdown/stream_equivalence_test.go`

Test helper sketch:

```go
func renderFull(input string) string {
    r := NewMarkdownRendererForWriter(io.Discard)
    return r(input)
}

func renderStreamed(input string, splits []int) string {
    var out strings.Builder
    sd := NewStepDisplay(&out)
    for _, chunk := range splitAt(input, splits) {
        sd.WriteText(chunk)
    }
    sd.End()
    return out.String()
}
```

### Phase 1: Move stream state out of StepDisplay

Create an internal stream coordinator:

- `terminal/ui/stream_markdown.go` initially, or
- `markdown/stream` if API stability is desired immediately.

`StepDisplay` should delegate assistant text to this component.

Acceptance criteria:

- No inline Markdown parsing helpers in `step.go`.
- Code fence detection is no longer duplicated between `step.go` and `markdown.go`.
- Existing tests pass.

### Phase 2: Paragraph-boundary correctness baseline

Implement a streaming parser that:

- Processes complete lines.
- Streams plain text paragraphs only when known to be plain, or emits paragraphs at boundaries.
- Streams fenced code lines after opening fence.
- Uses goldmark for finalized Markdown blocks/paragraphs.

Acceptance criteria:

- Full render equals streamed render for all test cases in the supported subset.
- No O(n²) reparsing for long code blocks.
- Long prose paragraphs have a documented latency behavior.

### Phase 3: CommonMark/GFM test corpus integration

Add conformance-oriented tests.

Renderer-level:

- Use CommonMark examples as inputs.
- Compare visible text or selected structural output.
- Avoid brittle ANSI exactness where not needed.

Streaming-level:

- For each selected input, split at many chunk boundaries.
- Assert output equivalence to unsplit streaming or full rendering.

Candidate tests:

- emphasis/strong delimiter examples
- code spans
- links and images
- autolinks
- fenced code blocks
- lists/blockquotes
- tables/task lists if GFM enabled

### Phase 4: Optional incremental inline parser

Only do this if Phase 2 latency is bad in real usage.

Implement:

- delimiter stack for `*` and `_`
- code span run matching
- link/image bracket stack
- raw HTML/autolink scanner
- rollback/holdback window for unresolved delimiters

Acceptance criteria:

- Pass selected CommonMark inline examples.
- Streaming split-fuzz tests pass.
- Simpler paragraph-boundary parser remains available behind a feature flag or test mode until confidence is high.

### Phase 5: Public API and migration cleanup

Decide whether `markdown.Buffer` remains, wraps the new parser, or is deprecated.

Options:

1. Keep `markdown.Buffer` as compatibility wrapper over new stream parser.
2. Add `markdown/stream` as new API and deprecate `Buffer` docs.
3. Keep `Buffer` for block buffering and make terminal UI use the new stream parser independently.

Recommendation: keep `Buffer` initially and add new APIs separately. Deprecate only after consumers have migrated.

## Trade-offs considered

### True incremental parser vs paragraph-boundary parser

True incremental parser:

- Pros: best responsiveness, least delayed rendering.
- Cons: high complexity, high test burden, easy CommonMark divergence.

Paragraph-boundary parser:

- Pros: simpler, goldmark remains source of truth, easier correctness story.
- Cons: long inline-styled paragraphs appear later.

Recommendation: paragraph-boundary baseline first; incremental inline only if proven necessary.

### Build vs dependency

Use existing Go dependency such as glamour:

- Pros: mature terminal rendering.
- Cons: not a true streaming parser, heavier dependency, style/layout behavior changes.

Build small streaming parser:

- Pros: precise control over streaming UX and append-only behavior.
- Cons: significant implementation/testing cost.

Recommendation: do not replace with glamour as Option D. Glamour can remain an optional renderer adapter later.

### Reusable package vs terminal-internal implementation

Reusable `markdown/stream`:

- Pros: clean architecture, usable by consumers.
- Cons: API design pressure too early.

Terminal-internal first:

- Pros: faster iteration, no premature public API.
- Cons: may need refactor later.

Recommendation: start terminal-internal unless there is an immediate external consumer.

## Explicitly out of scope

- Rewriting all terminal styling in the first pass.
- Perfect GFM table layout parity.
- HTML sanitization for terminal output beyond current behavior.
- Browser/DOM streaming rendering.
- Public API guarantee for experimental stream parser until tests prove it.

## Risks and mitigations

### Risk: accidental CommonMark divergence

Mitigation:

- Use goldmark for finalized block/paragraph rendering in Phase 2.
- Add CommonMark example tests before writing custom inline parser.

### Risk: worse perceived latency

Mitigation:

- Preserve code-fence line streaming.
- Add metrics/tests for first-byte-to-output behavior.
- Add optional paragraph force-flush only with clear documented trade-offs.

### Risk: O(n²) reparsing

Mitigation:

- Parse per finalized block/paragraph, not accumulated document.
- Stream code lines directly.
- Add benchmark for long paragraphs and long code blocks.

### Risk: public API churn

Mitigation:

- Keep experimental implementation internal first.
- Keep `markdown.Buffer` compatibility wrapper.

## Verification plan

Focused tests:

```bash
go test ./markdown ./terminal/ui
```

Full tests:

```bash
go test ./...
```

New test classes:

1. Full-render vs streamed-render equivalence.
2. Exhaustive split tests for short Markdown samples.
3. Randomized chunk tests for longer LLM-like responses.
4. CommonMark selected examples.
5. Benchmarks for long plain paragraphs and long fenced code blocks.

Benchmark sketch:

```bash
go test -bench=MarkdownStream ./terminal/ui ./markdown
```

Acceptance criteria for Option D completion:

- `StepDisplay` contains no Markdown delimiter parsing.
- Streaming parser owns all incomplete construct state.
- Supported Markdown subset is explicitly documented.
- Streamed render equals full render for supported test corpus across chunk boundaries.
- `go test ./...` passes.
- Benchmarks do not show O(n²) behavior on long responses.

## Open questions

1. Should the first implementation live under `terminal/ui/internal` or `markdown/stream`?
2. Is paragraph-level latency acceptable for real LLM responses?
3. Do we need exact ANSI snapshot tests or structural/visible-text tests?
4. Which GFM features are required for terminal UX: tables, task lists, strikethrough, autolinks?
5. Should `markdown.Buffer` be deprecated eventually, or kept as a lower-level block stabilization primitive?
