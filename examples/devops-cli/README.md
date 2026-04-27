# DevOps CLI

This example is a branded Go CLI built with `terminal/cli.NewCommand`. It embeds
agent resources and customizes the public CLI surface through `cli.Profile`.

Run it from this directory:

```bash
go run .
```

Run a one-shot task:

```bash
go run . "Draft an incident checklist for API latency after a deploy."
```

Because resources are embedded, this example is also a pattern for packaged
agent binaries. The `go.mod` file uses:

```go
replace github.com/codewandler/agentsdk => ../..
```

You can inspect the generated grouped CLI:

```bash
go run . --help
```
