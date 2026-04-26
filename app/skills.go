package app

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/codewandler/agentsdk/skill"
)

type SkillSourceDiscovery struct {
	WorkspaceDir               string
	HomeDir                    string
	Order                      int
	IncludeGlobalUserResources bool
}

func DiscoverDefaultSkillSources(cfg SkillSourceDiscovery) ([]skill.Source, error) {
	workspace := cfg.WorkspaceDir
	if workspace == "" {
		wd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("app: get working directory: %w", err)
		}
		workspace = wd
	}
	workspace, err := filepath.Abs(workspace)
	if err != nil {
		return nil, fmt.Errorf("app: resolve workspace skill source: %w", err)
	}

	type candidate struct {
		id    string
		label string
		dir   string
		kind  skill.SourceKind
	}
	candidates := []candidate{
		{id: "workspace:.agents/skills", label: "workspace .agents/skills", dir: filepath.Join(workspace, ".agents", "skills"), kind: skill.SourceAgentsCompat},
		{id: "workspace:.claude/skills", label: "workspace .claude/skills", dir: filepath.Join(workspace, ".claude", "skills"), kind: skill.SourceClaudeProject},
	}
	if cfg.IncludeGlobalUserResources {
		home := cfg.HomeDir
		if home == "" {
			home, err = os.UserHomeDir()
			if err != nil {
				return nil, fmt.Errorf("app: get home directory: %w", err)
			}
		}
		home, err = filepath.Abs(home)
		if err != nil {
			return nil, fmt.Errorf("app: resolve home skill source: %w", err)
		}
		candidates = append(candidates,
			candidate{id: "home:.agents/skills", label: "home .agents/skills", dir: filepath.Join(home, ".agents", "skills"), kind: skill.SourceAgentsCompat},
			candidate{id: "home:.claude/skills", label: "home .claude/skills", dir: filepath.Join(home, ".claude", "skills"), kind: skill.SourceClaudeUser},
		)
	}

	seen := map[string]bool{}
	sources := make([]skill.Source, 0, len(candidates))
	for _, candidate := range candidates {
		dir, err := filepath.Abs(candidate.dir)
		if err != nil {
			return nil, fmt.Errorf("app: resolve skill source %s: %w", candidate.dir, err)
		}
		if seen[dir] {
			continue
		}
		seen[dir] = true
		source := skill.DirSource(candidate.id, fmt.Sprintf("%s (%s)", candidate.label, dir), dir, candidate.kind, cfg.Order+len(sources))
		sources = append(sources, source)
	}
	return sources, nil
}
