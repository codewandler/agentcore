# Research Desk

This example builds its own Cobra CLI and constructs `app.App` directly instead
of using `terminal/cli.NewCommand`.

Run it from this directory:

```bash
go run . repl
```

Ask a one-shot question:

```bash
go run . ask "What are the tradeoffs of strict model compatibility routing?"
```

Digest pasted notes:

```bash
go run . digest "Source A says latency regressed after rollout. Source B says cache hit rate dropped."
```

The example embeds resources from `resources/.agents` and uses:

```go
replace github.com/codewandler/agentsdk => ../..
```

This is the pattern to use when you want agentsdk's app/agent/runtime building
blocks but need to own your CLI commands and application flow.
