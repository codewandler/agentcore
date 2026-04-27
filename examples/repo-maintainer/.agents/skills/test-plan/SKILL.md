---
name: test-plan
description: Convert a proposed code change into a focused, risk-based test plan.
---
# Test Plan Skill

Use this skill when the user asks how to verify a change or when a task affects
shared behavior.

Build test plans by risk, not by habit:

1. Identify the behavior contract being changed.
2. List the smallest unit tests that prove the new behavior.
3. Add integration or CLI tests only when cross-package wiring is affected.
4. Include one smoke command that exercises the user-facing path.
5. State which adjacent behavior should not be retested.
6. Call out cases where manual review is more valuable than more tests.
