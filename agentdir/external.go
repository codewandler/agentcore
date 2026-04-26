package agentdir

import (
	"os"
	"path/filepath"

	"github.com/codewandler/agentsdk/resource"
)

func appendExternalCandidates(bundle *resource.ContributionBundle, dir string, policy resource.DiscoveryPolicy) {
	if bundle == nil || !policy.IncludeExternalEcosystems {
		return
	}
	for _, c := range externalCandidates() {
		p := filepath.Join(dir, c.file)
		if _, err := os.Stat(p); err != nil {
			continue
		}
		source := resource.SourceRef{
			Ecosystem: "external",
			Scope:     resource.ScopeProject,
			Root:      dir,
			Path:      c.file,
			Trust:     resource.TrustUntrusted,
		}
		source.ID = resource.QualifiedID(source, "source", "", c.file)
		bundle.Tools = append(bundle.Tools, resource.ToolContribution{
			ID:          resource.QualifiedID(source, "tool", c.name, c.file),
			Name:        c.name,
			Description: c.desc,
			Source:      source,
			Enabled:     false,
		})
		bundle.Diagnostics = append(bundle.Diagnostics, resource.Info(source, c.desc))
	}
}

type externalCandidate struct {
	file string
	name string
	desc string
}

func externalCandidates() []externalCandidate {
	return []externalCandidate{
		{file: "Makefile", name: "make", desc: "Makefile targets detected; executable commands require explicit trust"},
		{file: "justfile", name: "just", desc: "justfile recipes detected; executable commands require explicit trust"},
		{file: "Taskfile.yml", name: "task", desc: "Taskfile tasks detected; executable commands require explicit trust"},
		{file: "Taskfile.yaml", name: "task", desc: "Taskfile tasks detected; executable commands require explicit trust"},
		{file: "package.json", name: "npm-scripts", desc: "package.json scripts detected; executable commands require explicit trust"},
		{file: "mcp.json", name: "mcp", desc: "MCP config detected; server connections require explicit trust"},
		{file: ".mcp.json", name: "mcp", desc: "MCP config detected; server connections require explicit trust"},
		{file: "openapi.yaml", name: "openapi", desc: "OpenAPI spec detected; tool generation requires explicit trust"},
		{file: "openapi.json", name: "openapi", desc: "OpenAPI spec detected; tool generation requires explicit trust"},
		{file: "AGENTS.md", name: "codex-agents", desc: "AGENTS.md detected; import requires an adapter decision"},
		{file: filepath.Join(".continue", "config.yaml"), name: "continue", desc: "Continue config detected; import requires an adapter decision"},
		{file: filepath.Join(".continue", "config.yml"), name: "continue", desc: "Continue config detected; import requires an adapter decision"},
	}
}
