// Package app composes agents, commands, plugins, and frontends.
package app

import (
	"github.com/codewandler/agentsdk/agent"
	"github.com/codewandler/agentsdk/command"
	"github.com/codewandler/agentsdk/skill"
	"github.com/codewandler/agentsdk/tool"
)

// Plugin is a named contribution bundle. Plugins may implement any of the
// optional contribution interfaces below.
type Plugin interface {
	Name() string
}

type CommandsPlugin interface {
	Plugin
	Commands() []command.Command
}

type AgentSpecsPlugin interface {
	Plugin
	AgentSpecs() []agent.Spec
}

type ToolsPlugin interface {
	Plugin
	Tools() []tool.Tool
}

type SkillsPlugin interface {
	Plugin
	SkillSources() []skill.Source
}
