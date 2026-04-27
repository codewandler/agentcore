---
name: devops
description: Advise on CI/CD pipelines, deployment strategies, and infrastructure automation.
---
# DevOps Skill

Use this skill for CI/CD pipeline design, deployment planning, infrastructure
automation, and production readiness review.

When advising on DevOps topics:

1. **Pipelines** — Keep build steps reproducible and cacheable. Separate build,
   test, lint, and deploy stages. Fail fast on the cheapest checks.
2. **Deployments** — Prefer rolling or blue-green deployments. Define rollback
   triggers before deploying. Include health checks and smoke tests.
3. **Infrastructure** — Treat infrastructure as code. Pin versions, use
   declarative configuration, and keep secrets out of repositories.
4. **Observability** — Every deployed service needs health endpoints, structured
   logs, and key metrics. Alert on symptoms, not causes.
5. **Security** — Apply least-privilege to service accounts, scan dependencies,
   and rotate credentials on a schedule.
6. **Reliability** — Identify single points of failure. Design for graceful
   degradation. Document recovery procedures.

Tie every recommendation to a concrete action: a config change, a command, or a
file to create. Avoid generic best-practice lists without project context.
