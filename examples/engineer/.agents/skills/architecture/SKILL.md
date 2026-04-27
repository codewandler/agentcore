---
name: architecture
description: Evaluate and design software architecture with clear trade-off analysis.
---
# Architecture Skill

Use this skill when the user asks about system design, component boundaries,
data flow, or technology selection.

When designing or evaluating architecture:

1. Start with the problem constraints: scale, latency, team size, deployment model.
2. Identify component boundaries and their communication contracts.
3. Prefer simple, well-understood patterns over novel ones.
4. Name trade-offs explicitly: consistency vs. availability, coupling vs. duplication,
   flexibility vs. complexity.
5. Separate stateless logic from stateful storage. Make state boundaries visible.
6. Design for observability: every component should expose health, metrics, and
   structured logs.
7. Call out what is deferred, not decided. Distinguish intentional simplicity from
   missing design.

Avoid architecture astronautics. Every abstraction must justify its cost with a
concrete scenario.
