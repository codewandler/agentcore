agentsdk> no, i wondered if there is a context provider for it ... I think what we need is a generic file content provider (or helper) - and then we have an agents.md context provider which would load that file, and update (re-render, diff) when changed ... create an in memory plan for this impl

-- Step 1/25 --------------------------------


You're right — the AGENTS.md content is currently baked into the system prompt via `initSkills()` / `materializedSystem`, not delivered through the context provider pipeline. That means it doesn't get diffed, doesn't get fingerprinted, doesn't survive compaction properly, and can't update mid-session if the file changes.

Here's the plan:

  --------

## Plan: File content provider and AGENTS.md context provider

### Step 1 —  FileProvider  helper in  contextproviders

A generic provider that reads one or more files from disk and renders each as a separate context fragment. On each  GetContext  call it re-reads the files, so the context manager's mechanical diff detects changes automatically.

    FileProvider
      key:       ProviderKey
      files:     []FileSpec

    FileSpec
      Path:      string              // absolute or relative to workdir
      Key:       FragmentKey          // fragment key (e.g. "agents_md/AGENTS.md")
      Role:      unified.Role         // default: user
      Authority: FragmentAuthority    // default: user
      Optional:  bool                 // skip silently if file doesn't exist

Core behavior:

•  GetContext  reads each file, trims whitespace, skips empty/missing optional files
• Each file becomes one  ContextFragment  with content = file body
• Fingerprint is computed from content (via existing  FragmentFingerprint )
• Implements  FingerprintingProvider  using file stat (mtime + size) as a cheap check — if stat hasn't changed, skip the full read
•  WithFileWorkDir  resolves relative paths
•  WithFileRunner  for testing (inject a  func(path) ([]byte, os.FileInfo, error) )

This is the file analog of  CmdProvider  — a reusable helper for any "read files, render as fragments" provider.

### Step 2 —  AgentsMarkdown  provider in  contextproviders

A thin wrapper that discovers and loads AGENTS.md / project instruction files as context fragments through the  FileProvider  helper.

    func AgentsMarkdown(paths []string, opts ...AgentsMarkdownOption) Provider

• Takes a list of discovered AGENTS.md paths (the caller already knows them from resource discovery)
• Each path becomes a  FileSpec  with  Key: "agents_md/" + sanitizedPath ,  Authority: user ,  Optional: true
• Re-reads on every  GetContext  call so edits mid-session are picked up on the next turn
• Uses the  FileProvider 's stat-based fingerprint fast path to avoid unnecessary reads when files haven't changed

### Step 3 — Wire into  agent.Instance.contextProviders()

Replace the current  ProjectInstructions  static provider (which snapshots content at init) with the new  AgentsMarkdown  provider that re-reads on each turn.

The agent already discovers AGENTS.md paths during resource resolution. Pass those paths to  AgentsMarkdown(paths)  in  contextProviders() .

### Step 4 — Remove AGENTS.md from  materializedSystem

Currently AGENTS.md content is concatenated into the system prompt string via  initSkills() . Once it's delivered through the context provider pipeline:

• It gets proper fingerprinting and diffing
• It survives compaction (context fragments are outside the compacted window)
• It can update mid-session if the file is edited
• It's visible in  /context  inspection
• It doesn't duplicate in the prompt (system prompt + context fragment)

This step is a breaking change — the system prompt gets shorter, and the content moves to context fragments. Need to verify no tests depend on AGENTS.md being in  materializedSystem .

### Dependencies and order

    Step 1 (FileProvider)     — no dependencies, pure helper + tests
    Step 2 (AgentsMarkdown)   — depends on Step 1
    Step 3 (wire into agent)  — depends on Step 2
    Step 4 (remove from system prompt) — depends on Step 3, needs careful test review

Steps 1-2 are safe and additive. Step 3 adds the provider. Step 4 removes the old path. Steps 3 and 4 could land together or separately.

### What this doesn't change

• Skills are still loaded via  initSkills()  and rendered by the  Skills  static provider. They could move to  FileProvider  later but that's a separate concern.
• Commands stay as-is — they're not context, they're tool definitions.
• The  ProjectInstructions  static provider in  contextproviders/static.go  stays available for callers who want to inject instruction content without file paths.

  --------

Want me to start with Step 1?
-- step 1 -- in: 235k (cache_w: 233k  new: 1.8k)  out: 1.1k  cost: $1.49  model: opus
dims: provider=claude model=claude-opus-4-6 turn_id=14 session_id=WAtq2uu8
usage: total_input=234920 input=1841 cache_write=233079 total_output=1119
costs: total=1.493924
agentsdk> you should also include into this change that after the first initial rendering of the context that particular message must ensure caching, eg figure out if we lose any caching capabilities by doing this change

-- Step 1/25 --------------------------------


Good call. Let me think through the caching implications before writing code.

The current path — AGENTS.md baked into `materializedSystem` which becomes a system instruction — is naturally cache-friendly because system instructions are at the top of the prompt and don't change between turns. Providers like Anthropic and OpenAI cache prompt prefixes, so a stable system instruction gets cached automatically.

If we move AGENTS.md to context fragments injected as `Items` (user role) or `Instructions` (developer authority), we need to ensure:

1. Stable position — fragments must appear in the same position in the prompt across turns
2. Cache control hints — fragments should carry  CachePolicy{Stable: true, Scope: CacheThread}  so the context manager knows they're long-lived
3. Provider-level cache breakpoints — Anthropic's API supports  cache_control  on content parts; we need to check if the existing injection path preserves this

Let me trace how cache control flows through the system:

> tool: grep
{
"paths": [
"runtime/thread_runtime.go",
"runtime/history.go",
"conversation/projection_policy.go"
],
"pattern": "CacheControl|cache_control|CachePolic",
"show_content": true
}
-- step 1 -- in: 236k (cache_r: 234k 99.0%  cache_w: 925  new: 1.4k)  out: 355  cost: $0.1388  model: opus
dims: provider=claude model=claude-opus-4-6 turn_id=15 session_id=WAtq2uu8
usage: total_input=236080 input=1449 cache_read=233706 cache_write=925 total_output=355
costs: total=0.138754
ok runtime/thread_runtime.go:
311:  CachePolicy agentcontext.CachePolicy `json:"cache_policy,omitempty"`
351:          CachePolicy: payload.CachePolicy,
429:                  CachePolicy: fragment.CachePolicy,
runtime/history.go:
40:   cachePolicy     unified.CachePolicy
53:           defaults:  historyDefaults{cachePo...

-- Step 2/25 --------------------------------

> tool: file_read
{
"path": "runtime/history.go",
"ranges": [
{
"end": 280,
"start": 240
}
]
}
-- step 2 -- in: 237k (cache_r: 235k 99.2%  cache_w: 21  new: 2.0k)  out: 87  cost: $0.1294  model: opus
dims: provider=claude model=claude-opus-4-6 turn_id=15 session_id=WAtq2uu8
usage: total_input=236642 input=1961 cache_read=234660 cache_write=21 total_output=87
costs: total=0.129441
ok [File: /home/timo/projects/codewandler-ai/agentsdk/runtime/history.go] [Lines: 449 total] [Size: 12.8KB] [Ranges: 240-280]

240│            MaxOutputTokens: firstIntPtr(req.MaxOutputTokens, h.defaults.maxOutputTokens),
241│            Temperature:     firstFloatPtr(req.Temperature, h.defaults.temperature),
242...

-- Step 3/25 --------------------------------
Now let me check how context fragments are injected — specifically whether `CacheControl` is set on the content parts:

> tool: file_read
{
"path": "runtime/thread_runtime.go",
"ranges": [
{
"end": 715,
"start": 661
}
]
}
-- step 3 -- in: 238k (cache_r: 235k 98.8%  cache_w: 1.3k  new: 1.5k)  out: 115  cost: $0.1358  model: opus
dims: provider=claude model=claude-opus-4-6 turn_id=15 session_id=WAtq2uu8
usage: total_input=237572 input=1534 cache_read=234775 cache_write=1263 total_output=115
costs: total=0.135826
ok [File: /home/timo/projects/codewandler-ai/agentsdk/runtime/thread_runtime.go] [Lines: 755 total] [Size: 23.1KB] [Ranges: 661-715]

661│ type contextInjection struct {
662│    Instructions []unified.Instruction
663│    Items        []conversation.Item
664│ }
665│
666│ func contextInjectio...

-- Step 4/25 --------------------------------
There it is — line 689: `unified.TextPart{Text: content}` with **no `CacheControl`**. The fragment's `CachePolicy` is stored in the render record but never translated to a `CacheControl` hint on the content part. Let me check what `unified.CacheControl` looks like:

> tool: bash
{
"cmd": "cd /home/timo/projects/codewandler-ai/agentsdk \u0026\u0026 grep -r 'CacheControl' $(go env GOMODCACHE)/github.com/codewandler/llmadapter@v0.37.0/unified/ --include='*.go' | head -15"
}
-- step 4 -- in: 239k (cache_r: 236k 99.0%  cache_w: 358  new: 2.1k)  out: 196  cost: $0.1356  model: opus
dims: provider=claude model=claude-opus-4-6 turn_id=15 session_id=WAtq2uu8
usage: total_input=238513 input=2078 cache_read=236077 cache_write=358 total_output=196
costs: total=0.135566
ok [exit: 0] [duration: 0.0s] [dir: /home/timo/projects/codewandler-ai/agentsdk]
=== STDOUT ===
/home/timo/go/pkg/mod/github.com/codewandler/llmadapter@v0.37.0/unified/cache.go:type CacheControlType string
/home/timo/go/pkg/mod/github.com/codewandler/llmadapter@v0.37.0/unified/cache.go:       CacheControl...

-- Step 5/25 --------------------------------

> tool: bash
{
"cmd": "cat $(go env GOMODCACHE)/github.com/codewandler/llmadapter@v0.37.0/unified/cache.go"
}
-- step 5 -- in: 239k (cache_r: 235k 98.2%  cache_w: 1.2k  new: 3.2k)  out: 86  cost: $0.1429  model: opus
dims: provider=claude model=claude-opus-4-6 turn_id=15 session_id=WAtq2uu8
usage: total_input=239144 input=3182 cache_read=234775 cache_write=1187 total_output=86
costs: total=0.142866
ok [exit: 0] [duration: 0.0s] [dir: /home/timo/projects/codewandler-ai/agentsdk]
=== STDOUT ===
package unified

type CacheControlType string

const (
CacheControlEphemeral CacheControlType = "ephemeral"
)

type CacheControl struct {
Type CacheControlType `json:"type"`
TTL  string           `json...

-- Step 6/25 --------------------------------

> tool: bash
{
"cmd": "grep -A5 'type TextPart' $(go env GOMODCACHE)/github.com/codewandler/llmadapter@v0.37.0/unified/content.go"
}
-- step 6 -- in: 239k (cache_r: 235k 98.1%  cache_w: 1.2k  new: 3.4k)  out: 96  cost: $0.1444  model: opus
dims: provider=claude model=claude-opus-4-6 turn_id=15 session_id=WAtq2uu8
usage: total_input=239401 input=3439 cache_read=234775 cache_write=1187 total_output=96
costs: total=0.144401
ok [exit: 0] [duration: 0.0s] [dir: /home/timo/projects/codewandler-ai/agentsdk]
=== STDOUT ===
type TextPart struct {
Text         string        `json:"text,omitempty"`
CacheControl *CacheControl `json:"cache_control,omitempty"`
}

func (TextPart) contentKind() ContentKind { return ContentKindText }

-- Step 7/25 --------------------------------
Now let me check how the request-level `CachePolicy` interacts with content-level `CacheControl`:

> tool: bash
{
"cmd": "grep -r 'CachePolicy\\|cache_policy' $(go env GOMODCACHE)/github.com/codewandler/llmadapter@v0.37.0/unified/ --include='*.go' | head -10"
}
-- step 7 -- in: 240k (cache_r: 236k 98.5%  cache_w: 2.7k  new: 909)  out: 132  cost: $0.1427  model: opus
dims: provider=claude model=claude-opus-4-6 turn_id=15 session_id=WAtq2uu8
usage: total_input=239618 input=909 cache_read=236016 cache_write=2693 total_output=132
costs: total=0.142684
ok [exit: 0] [duration: 0.0s] [dir: /home/timo/projects/codewandler-ai/agentsdk]

-- Step 8/25 --------------------------------

> tool: bash
{
"cmd": "grep -r 'CachePolicy' $(go env GOMODCACHE)/github.com/codewandler/llmadapter@v0.37.0/unified/ --include='*.go' | head -10"
}
-- step 8 -- in: 240k (cache_r: 239k 99.7%  cache_w: 87  new: 568)  out: 101  cost: $0.1255  model: opus
dims: provider=claude model=claude-opus-4-6 turn_id=15 session_id=WAtq2uu8
usage: total_input=239798 input=568 cache_read=239143 cache_write=87 total_output=101
costs: total=0.125480
ok [exit: 0] [duration: 0.0s] [dir: /home/timo/projects/codewandler-ai/agentsdk]

-- Step 9/25 --------------------------------

> tool: bash
{
"cmd": "grep -r 'CachePolicy' $(go env GOMODCACHE)/github.com/codewandler/llmadapter@v0.37.0/ --include='*.go' | head -10"
}
-- step 9 -- in: 240k (cache_r: 239k 99.8%  cache_w: 97  new: 450)  out: 99  cost: $0.1250  model: opus
dims: provider=claude model=claude-opus-4-6 turn_id=15 session_id=WAtq2uu8
usage: total_input=239947 input=450 cache_read=239400 cache_write=97 total_output=99
costs: total=0.125031
ok [exit: 0] [duration: 0.0s] [dir: /home/timo/projects/codewandler-ai/agentsdk]

-- Step 10/25 --------------------------------

> tool: bash
{
"cmd": "grep -ri 'cachepolicy\\|cache_policy' $(go env GOMODCACHE)/github.com/codewandler/llmadapter@v0.37.0/ --include='*.go' | head -10"
}
-- step 10 -- in: 240k (cache_r: 240k 99.8%  cache_w: 133  new: 344)  out: 104  cost: $0.1250  model: opus
dims: provider=claude model=claude-opus-4-6 turn_id=15 session_id=WAtq2uu8
usage: total_input=240094 input=344 cache_read=239617 cache_write=133 total_output=104
costs: total=0.124960
ok [exit: 0] [duration: 0.0s] [dir: /home/timo/projects/codewandler-ai/agentsdk]

-- Step 11/25 --------------------------------

> tool: bash
{
"cmd": "grep -ri 'cache' $(go env GOMODCACHE)/github.com/codewandler/llmadapter@v0.37.0/unified/request.go | head -10"
}
-- step 11 -- in: 240k (cache_r: 240k 99.8%  cache_w: 102  new: 347)  out: 96  cost: $0.1247  model: opus
dims: provider=claude model=claude-opus-4-6 turn_id=15 session_id=WAtq2uu8
usage: total_input=240246 input=347 cache_read=239797 cache_write=102 total_output=96
costs: total=0.124671
ok [exit: 0] [duration: 0.0s] [dir: /home/timo/projects/codewandler-ai/agentsdk]

-- Step 12/25 --------------------------------

> tool: bash
{
"cmd": "grep 'Cache' $(go env GOMODCACHE)/github.com/codewandler/llmadapter@v0.37.0/unified/request.go"
}
-- step 12 -- in: 240k (cache_r: 240k 99.8%  cache_w: 100  new: 344)  out: 90  cost: $0.1246  model: opus
dims: provider=claude model=claude-opus-4-6 turn_id=15 session_id=WAtq2uu8
usage: total_input=240390 input=344 cache_read=239946 cache_write=100 total_output=90
costs: total=0.124568
err [exit: 1] [duration: 0.0s] [dir: /home/timo/projects/codewandler-ai/agentsdk]

-- Step 13/25 --------------------------------

> tool: bash
{
"cmd": "grep -ri 'cache' $(go env GOMODCACHE)/github.com/codewandler/llmadapter@v0.37.0/unified/*.go | grep -v '_test.go' | head -20"
}
-- step 13 -- in: 241k (cache_r: 240k 99.8%  cache_w: 105  new: 338)  out: 103  cost: $0.1250  model: opus
dims: provider=claude model=claude-opus-4-6 turn_id=15 session_id=WAtq2uu8
usage: total_input=240536 input=338 cache_read=240093 cache_write=105 total_output=103
costs: total=0.124968
ok [exit: 0] [duration: 0.0s] [dir: /home/timo/projects/codewandler-ai/agentsdk]
=== STDOUT ===
/home/timo/go/pkg/mod/github.com/codewandler/llmadapter@v0.37.0/unified/cache.go:type CacheControlType string
/home/timo/go/pkg/mod/github.com/codewandler/llmadapter@v0.37.0/unified/cache.go:       CacheControl...

-- Step 14/25 --------------------------------
Now let me check how the current system prompt path handles caching — specifically the `CachePolicy` on the request:

> tool: grep
{
"paths": [
"runtime/history.go",
"runtime/options.go"
],
"pattern": "CachePolicy|CacheKey|CacheTTL",
"show_content": true
}
-- step 14 -- in: 242k  out: 138  cost: $1.21  model: opus
dims: provider=claude model=claude-opus-4-6 turn_id=15 session_id=WAtq2uu8
usage: total_input=241743 input=241743 total_output=138
costs: total=1.212165
ok runtime/history.go:
40:   cachePolicy     unified.CachePolicy
53:           defaults:  historyDefaults{cachePolicy: unified.CachePolicyOn},
152: func WithHistoryCachePolicy(policy unified.CachePolicy) HistoryOption {
156: func WithHistoryCacheKey(key string) HistoryOption {
160: func WithHistory...

-- Step 15/25 --------------------------------

> tool: bash
{
"cmd": "grep -r 'type CachePolicy\\|CachePolicyOn\\|CachePolicyOff' $(go env GOMODCACHE)/github.com/codewandler/llmadapter@v0.37.0/ --include='*.go' | head -10"
}
-- step 15 -- in: 242k (cache_w: 241k  new: 1.7k)  out: 115  cost: $1.52  model: opus
dims: provider=claude model=claude-opus-4-6 turn_id=15 session_id=WAtq2uu8
usage: total_input=242324 input=1685 cache_write=240639 total_output=115
costs: total=1.515294
ok [exit: 0] [duration: 0.0s] [dir: /home/timo/projects/codewandler-ai/agentsdk]

-- Step 16/25 --------------------------------

> tool: bash
{
"cmd": "grep -r 'CachePolicy' $(go env GOMODCACHE)/github.com/codewandler/llmadapter@v0.37.0/ --include='*.go' -l"
}
-- step 16 -- in: 242k (cache_w: 241k  new: 1.8k)  out: 97  cost: $1.52  model: opus
dims: provider=claude model=claude-opus-4-6 turn_id=15 session_id=WAtq2uu8
usage: total_input=242487 input=1848 cache_write=240639 total_output=97
costs: total=1.515659
err [exit: 1] [duration: 0.0s] [dir: /home/timo/projects/codewandler-ai/agentsdk]
