---
name: code-review
description: Review code changes for correctness, clarity, and maintainability.
---
# Code Review Skill

Use this skill when the user asks for a code review, feedback on a diff, or
guidance on improving existing code.

Review checklist:

1. **Correctness** — Does the code do what it claims? Check edge cases, error
   handling, and off-by-one conditions.
2. **Clarity** — Can a new team member understand the intent without extra context?
   Flag unclear names, implicit assumptions, and magic values.
3. **Maintainability** — Is the change easy to modify later? Watch for tight
   coupling, duplicated logic, and missing abstractions.
4. **Testing** — Are the important behaviors covered? Identify untested paths and
   suggest the smallest useful tests.
5. **Performance** — Flag only concrete concerns: unnecessary allocations in hot
   paths, O(n²) where O(n) is straightforward, missing indexes.
6. **Style** — Follow the project's existing conventions. Do not impose external
   style preferences.

Deliver feedback as actionable items. For each issue, state the file, the
concern, and a concrete suggestion. Separate blocking issues from nits.
