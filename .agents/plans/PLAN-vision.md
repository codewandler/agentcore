# Plan: Vision plugin — image understanding via OpenRouter vision models

## Problem statement

Agents currently have no way to analyze images. When a user references a
screenshot, diagram, or photo, the agent cannot inspect it. A `vision` tool
would let the LLM delegate image understanding to a dedicated vision model,
returning a textual description or structured analysis.

## Goal

Add a `vision` tool exposed through a `visionplugin` plugin. The tool accepts
an image source (URL, local file path, or base64 data) and a prompt, sends it
to a vision-capable model via OpenRouter (using the existing `llmadapter`
unified client), and returns the model's textual response.

## Design

### Tool surface

```
vision([{action: "understand", images: ["<url|path|base64>", ...], prompt: "optional prompt"}])
```

Single action for now (`understand`). The action-array pattern keeps the door
open for future actions (e.g. `ocr`, `compare`, `extract_structured`) without
changing the tool schema.

**Parameters:**

| Field    | Type   | Required | Description                                                        |
|----------|--------|----------|--------------------------------------------------------------------|
| `action` | string | yes      | Action to perform. Currently only `"understand"`.                  |
| `images` | []string | yes    | Image sources: HTTP(S) URLs, local file paths, or `data:` base64 URIs.|
| `prompt` | string | no       | What to focus on. Defaults to a general description prompt.        |

**Result:** Plain text description returned by the vision model, wrapped in a
standard `tool.Result`.

### Image source resolution

The tool resolves the `image` parameter in order:

1. **URL** — starts with `http://` or `https://`. Passed as-is via
   `unified.BlobSourceURL`.
2. **Data URI** — starts with `data:image/`. Decoded to base64 + media type,
   passed via `unified.BlobSourceBase64`.
3. **File path** — anything else. Resolved relative to `ctx.WorkDir()`. Read
   from disk, base64-encoded, media type inferred from extension. Passed via
   `unified.BlobSourceBase64`. Capped at 20 MB to avoid OOM.

### Vision model selection

The tool needs its own LLM client because the agent's primary model may not
support vision, and the vision call is a sub-request (tool implementation
detail), not part of the main conversation.

**Configuration (in priority order):**

1. Model is hardcoded: `google/gemini-2.5-flash`.
2. `VISION_OPENROUTER_API_KEY` env var — dedicated key for vision calls.
   Falls back to `OPENROUTER_API_KEY`.

The tool creates its own `unified.Client` via `llmadapter/adapt` at
construction time, configured for OpenRouter. This is independent of the
agent's main client.

### Package layout

```
tools/vision/
├── vision.go          # Tool implementation, image resolution, LLM call
└── vision_test.go     # Unit tests (mock client, source resolution)

plugins/visionplugin/
├── visionplugin.go    # Plugin wrapper (app.Plugin + ToolsPlugin)
└── visionplugin_test.go
```

Following the established pattern: low-level tool in `tools/vision/` (usable
standalone), plugin wrapper in `plugins/visionplugin/` (for `app.App` users).

### Plugin interface

The vision plugin is a pure `ToolsPlugin` — no context providers, no commands,
no skills. Minimal surface:

```go
// plugins/visionplugin/visionplugin.go
package visionplugin

type Plugin struct {
    client unified.Client
    model  string
}

func New(opts ...Option) *Plugin { ... }
func (p *Plugin) Name() string          { return "vision" }
func (p *Plugin) Tools() []tool.Tool    { return vision.Tools(p.client, p.model) }
```

Options: `WithClient(unified.Client)`, `WithAPIKey(string)`.

### Tool implementation sketch

```go
// tools/vision/vision.go
package vision

type Action struct {
    Action string   `json:"action" jsonschema:"description=Action to perform,enum=understand,required"`
    Images []string `json:"images" jsonschema:"description=Image sources: URLs or file paths or data URIs,required"`
    Prompt string   `json:"prompt,omitempty" jsonschema:"description=What to analyze or describe"`
}

type Params struct {
    Actions []Action `json:"actions" jsonschema:"description=Vision actions to perform,required"`
}

func Tools(client unified.Client, model string) []tool.Tool {
    return []tool.Tool{visionTool(client, model)}
}

func visionTool(client unified.Client, model string) tool.Tool {
    return tool.New("vision",
        "Analyze images using a vision model. Supports URLs, file paths, and data URIs.",
        func(ctx tool.Ctx, p Params) (tool.Result, error) {
            // Process each action, collect results
            // For each action:
            //   1. Resolve image source → unified.ImagePart
            //   2. Build unified.Request with image + prompt
            //   3. Call client.Request(), collect response
            //   4. Return text content as result
        },
    )
}
```

### Image source resolution details

```go
func resolveImage(workDir, image string) (unified.ImagePart, error) {
    switch {
    case strings.HasPrefix(image, "http://"), strings.HasPrefix(image, "https://"):
        return unified.ImagePart{
            Source: unified.BlobSource{Kind: unified.BlobSourceURL, URL: image},
        }, nil

    case strings.HasPrefix(image, "data:"):
        mediaType, b64, err := parseDataURI(image)
        // ...
        return unified.ImagePart{
            Source: unified.BlobSource{
                Kind: unified.BlobSourceBase64, Base64: b64, MIMEType: mediaType,
            },
        }, nil

    default: // file path
        absPath := filepath.Join(workDir, image) // resolve relative
        data, err := os.ReadFile(absPath)
        // ... size check, media type from extension
        return unified.ImagePart{
            Source: unified.BlobSource{
                Kind: unified.BlobSourceBase64,
                Base64: base64.StdEncoding.EncodeToString(data),
                MIMEType: mimeFromExt(absPath),
            },
        }, nil
    }
}
```

### LLM request construction

```go
req := unified.Request{
    Model:  model,
    Stream: false,
    Messages: []unified.Message{{
        Role: unified.RoleUser,
        Content: []unified.ContentPart{
            imagePart,
            unified.TextPart{Text: prompt},
        },
    }},
}
events, err := client.Request(ctx, req)
resp, err := unified.Collect(ctx, events)
// Extract text from resp.Content
```

Non-streaming is fine here — the vision call is a tool sub-request, not a
user-facing stream. The agent loop will show the tool result once complete.

## Trade-offs

### Own client vs. reusing agent client

**Chosen: own client.** The agent's primary model may not support vision (e.g.
Claude via Anthropic API doesn't accept image URLs the same way OpenRouter
does). A dedicated client also lets us pin a cost-effective vision model
independently of the conversation model.

**Downside:** Extra configuration surface (API key, model). Mitigated by
sensible defaults and env var fallbacks.

### Single action vs. direct tool

**Chosen: action array.** Matches the pattern used by other tools in the SDK
(e.g. `skill`). Allows batching multiple images in one call and adding future
actions without schema changes.

**Downside:** Slightly more complex schema for a single-action tool today.

### OpenRouter-specific vs. generic provider

**Chosen: OpenRouter via llmadapter.** OpenRouter gives access to many vision
models (Gemini, GPT-4o, Llama, etc.) through a single API key. Using
`llmadapter` means we get the unified client abstraction and can later swap
providers without changing tool code.

**Downside:** Requires an OpenRouter API key. Users who only have Anthropic or
OpenAI keys would need to configure OpenRouter separately. Could be generalized
later by accepting any `unified.Client`.

### Default model choice

**Chosen: `google/gemini-2.5-flash`.** Hardcoded. Fast, cheap, strong vision
capabilities, widely available on OpenRouter. No configurability needed for v1.

## Implementation steps

### Step 1: `tools/vision/` — core tool implementation

- [ ] `vision.go` — `Params`, `Action` types, `Tools()` factory, image source
  resolution, LLM request/response handling.
- [ ] `vision_test.go` — test image source resolution (URL, data URI, file
  path), test tool execution with mock client, test error cases (missing image,
  unsupported action, file too large, file not found).

### Step 2: `plugins/visionplugin/` — plugin wrapper

- [ ] `visionplugin.go` — `Plugin` struct, `New()` with options, `Name()`,
  `Tools()`. Handles client construction from env vars / options.
- [ ] `visionplugin_test.go` — test plugin wiring, option application.

### Step 3: Wire into `plugins/standard/` (optional)

- [ ] Add `IncludeVision bool` to `standard.Options`.
- [ ] Conditionally include `visionplugin.New()` in `Plugins()`.
- [ ] Update `standard_test.go`.

**Decision needed:** Vision requires an external API key and adds a network
dependency. It may be better to keep it opt-in only (not in standard set) and
let consumers add it explicitly. Leaning toward **not** adding to standard —
consumers wire it themselves:

```go
app.New(
    app.WithPlugins(
        standard.DefaultPlugins()...,
    ),
    app.WithPlugins(
        visionplugin.New(),
    ),
)
```

### Step 4: Documentation and examples

- [ ] Add guidance string to the tool for HEAD context.
- [ ] Update `AGENTS.md` plugin list.
- [ ] Consider adding vision to an existing example or creating a minimal one.

## Environment variables

| Variable                     | Purpose                              | Default                      |
|------------------------------|--------------------------------------|------------------------------|
| `VISION_OPENROUTER_API_KEY`  | Dedicated API key for vision calls   | falls back to `OPENROUTER_API_KEY` |
| `OPENROUTER_API_KEY`         | Shared OpenRouter API key            | (none — tool reports config error) |

## Open questions

1. ~~**Max image size for file reads?**~~ **Decided: 10 MB.** Large enough for
   screenshots, small enough to avoid OOM.
2. ~~**Multiple images per action?**~~ **Decided: yes.** `images: []string`
   allows sending multiple images in a single action for comparison, context,
   etc.
3. ~~**Response token limit?**~~ **Decided: yes, 4096 tokens default.**
4. ~~**Should the tool validate that the model actually supports vision?**~~
   **Decided: no.** Model is hardcoded to a known vision-capable model.

## Non-goals (v1)

- Video understanding (future action).
- Image generation (different tool entirely).
- OCR as a separate action (can be done via prompt: "extract all text").
- Caching/deduplication of repeated image analyses.
- Streaming the vision model response to the user.
