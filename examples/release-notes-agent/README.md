# Release Notes Agent

This is the smallest useful agentsdk example: a filesystem-described agent with
only a system prompt.

Run it from the agentsdk repository root:

```bash
go run ./cmd/agentsdk run examples/release-notes-agent
```

Example prompt:

```text
Read the recent git history and draft release notes grouped by feature, fix, and docs.
```

If you have an installed `agentsdk` binary:

```bash
agentsdk run examples/release-notes-agent
```
