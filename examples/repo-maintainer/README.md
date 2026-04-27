# Repo Maintainer

This example shows a pure resource bundle with an agent, a slash command, and a
skill. It does not contain Go code.

Run it from the agentsdk repository root:

```bash
go run ./cmd/agentsdk run examples/repo-maintainer
```

Example prompts:

```text
/triage The test suite is flaky around session resume after model routing changes.
```

```text
Create a focused test plan for changing CLI flag parsing.
```

If you have an installed `agentsdk` binary:

```bash
agentsdk run examples/repo-maintainer
```
