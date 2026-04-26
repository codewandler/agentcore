# Resource Format References

This file tracks external resource-format references used by agentsdk design
and implementation decisions.

## Claude Code

- Claude Code subagents: https://docs.claude.com/en/docs/claude-code/subagents
- Claude Code slash commands: https://docs.claude.com/en/docs/claude-code/slash-commands
- Claude Code plugins: https://docs.claude.com/en/docs/claude-code/plugins
- Claude Code skills: https://docs.claude.com/en/docs/claude-code/skills

Relevant compatibility layouts:

```text
.claude/
  agents/
  commands/
  skills/

plugin/
  .claude-plugin/
    plugin.json
  agents/
  commands/
  skills/
```

## Agent Skills

- OpenCode skills: https://opencode.ai/docs/skills

Relevant compatibility layouts:

```text
.claude/skills/<skill>/SKILL.md
.agents/skills/<skill>/SKILL.md
~/.claude/skills/<skill>/SKILL.md
~/.agents/skills/<skill>/SKILL.md
```

`SKILL.md` directories are the canonical skill format for agentsdk. agentsdk
loads `.agents/skills` as a skill compatibility source, but `.agents/agents`
and `.agents/commands` are not ambient default layouts.

## Project Instructions

- AGENTS.md: https://agents.md/

`AGENTS.md` is a project instruction convention. It is not an agent, command,
skill, or plugin bundle format.
