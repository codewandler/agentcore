---
description: Create a deployment checklist for a service or change.
argument-hint: "<service, release, or change to deploy>"
---
Create a deployment checklist for:

{{.Query}}

Include:

- pre-deployment checks (tests green, dependencies pinned, config reviewed)
- deployment steps in order
- health checks and smoke tests to run after deployment
- rollback criteria and procedure
- monitoring to watch during and after rollout
- communication checkpoints (team, stakeholders, on-call)
- post-deployment cleanup or follow-up tasks
