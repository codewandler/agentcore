# Engineer

A senior software engineer agent with skills in architecture, code review, and
DevOps. It does not contain Go code — it is a pure resource bundle.

Run it from the agentsdk repository root:

```bash
go run ./cmd/agentsdk run examples/engineer
```

## Commands

| Command   | Description                                                  |
| --------- | ------------------------------------------------------------ |
| `/review` | Review code changes for correctness, clarity, maintainability |
| `/design` | Produce a lightweight architecture design for a feature       |
| `/deploy` | Create a deployment checklist for a service or change         |

## Skills

| Skill          | Description                                              |
| -------------- | -------------------------------------------------------- |
| architecture   | System design, component boundaries, trade-off analysis  |
| code-review    | Structured code review with actionable feedback          |
| devops         | CI/CD pipelines, deployment strategies, infrastructure   |

## Example prompts

```text
/review the changes in src/auth/session.go — focus on error handling
```

```text
/design a rate-limiting layer in front of the public API
```

```text
/deploy the billing service v2.4.0 to production
```

```text
What are the trade-offs between an event-driven and a request-driven architecture for our notification system?
```

If you have an installed `agentsdk` binary:

```bash
agentsdk run examples/engineer
```
