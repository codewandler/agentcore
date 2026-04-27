---
description: Review code changes for correctness, clarity, and maintainability.
argument-hint: "<file, diff, or description of changes>"
---
Review the following code changes:

{{.Query}}

Provide:

- a one-sentence summary of what the change does
- blocking issues (correctness, security, data loss)
- maintainability concerns (coupling, naming, missing abstractions)
- untested paths and suggested test cases
- nits and style suggestions (separate from blocking issues)
- an overall assessment: approve, request changes, or needs discussion
