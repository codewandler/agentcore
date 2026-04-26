package agentdir

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/codewandler/agentsdk/agent"
)

var manifestNames = []string{"app.manifest.json", "agentsdk.app.json"}

type AppManifest struct {
	DefaultAgent string           `json:"default_agent"`
	Plugins      []ManifestPlugin `json:"plugins"`
}

type ManifestPlugin struct {
	Path string `json:"path"`
}

type Resolution struct {
	Bundle       Bundle
	DefaultAgent string
	Manifest     *AppManifest
	Sources      []string
}

// ResolveDir resolves a path as an app manifest, embedded plugin roots, or a
// plugin root in the deterministic order used by agentsdk run.
func ResolveDir(dir string) (Resolution, error) {
	manifestPath, manifest, ok, err := readManifest(dir)
	if err != nil {
		return Resolution{}, err
	}
	if ok {
		return resolveManifest(dir, manifestPath, manifest)
	}
	var out Resolution
	for _, name := range []string{".claude", ".agents"} {
		candidate := filepath.Join(dir, name)
		if exists, err := osDirExists(candidate); err != nil {
			return Resolution{}, err
		} else if exists {
			bundle, err := LoadDir(candidate)
			if err != nil {
				return Resolution{}, fmt.Errorf("load %s: %w", candidate, err)
			}
			if err := out.Bundle.Append(bundle); err != nil {
				return Resolution{}, err
			}
			out.Sources = append(out.Sources, candidate)
		}
	}
	if len(out.Sources) > 0 {
		return out, nil
	}
	bundle, err := LoadDir(dir)
	if err != nil {
		return Resolution{}, err
	}
	out.Bundle = bundle
	out.Sources = append(out.Sources, dir)
	return out, nil
}

// ResolveFS resolves an embedded or virtual filesystem root as a plugin root.
func ResolveFS(fsys fs.FS, root string) (Resolution, error) {
	bundle, err := LoadFS(fsys, root)
	if err != nil {
		return Resolution{}, err
	}
	return Resolution{Bundle: bundle, Sources: []string{root}}, nil
}

func ResolveDefaultAgent(specs []string, explicit string, manifestDefault string) (string, error) {
	names := append([]string(nil), specs...)
	sort.Strings(names)
	has := func(name string) bool {
		for _, candidate := range names {
			if candidate == name {
				return true
			}
		}
		return false
	}
	if explicit != "" {
		if !has(explicit) {
			return "", fmt.Errorf("agentdir: agent %q not found; available agents: %v", explicit, names)
		}
		return explicit, nil
	}
	if manifestDefault != "" {
		if !has(manifestDefault) {
			return "", fmt.Errorf("agentdir: default agent %q not found; available agents: %v", manifestDefault, names)
		}
		return manifestDefault, nil
	}
	if len(names) == 1 {
		return names[0], nil
	}
	for _, conventional := range []string{"main", "default"} {
		if has(conventional) {
			return conventional, nil
		}
	}
	if len(names) == 0 {
		return "", fmt.Errorf("agentdir: no agents found")
	}
	return "", fmt.Errorf("agentdir: multiple agents found; choose one with --agent: %v", names)
}

func (r Resolution) AgentNames() []string {
	names := make([]string, 0, len(r.Bundle.AgentSpecs))
	for _, spec := range r.Bundle.AgentSpecs {
		names = append(names, spec.Name)
	}
	sort.Strings(names)
	return names
}

func (r Resolution) ResolveDefaultAgent(explicit string) (string, error) {
	return ResolveDefaultAgent(r.AgentNames(), explicit, r.DefaultAgent)
}

func (r *Resolution) UpdateAgentSpec(name string, update func(*agent.Spec)) error {
	if r == nil {
		return fmt.Errorf("agentdir: resolution is nil")
	}
	if name == "" {
		return fmt.Errorf("agentdir: agent name is required")
	}
	for i := range r.Bundle.AgentSpecs {
		if r.Bundle.AgentSpecs[i].Name == name {
			if update != nil {
				update(&r.Bundle.AgentSpecs[i])
			}
			return nil
		}
	}
	return fmt.Errorf("agentdir: agent spec %q not found; available agents: %v", name, r.AgentNames())
}

func resolveManifest(dir string, manifestPath string, manifest AppManifest) (Resolution, error) {
	out := Resolution{Manifest: &manifest, DefaultAgent: manifest.DefaultAgent, Sources: []string{manifestPath}}
	for _, plugin := range manifest.Plugins {
		if plugin.Path == "" {
			return Resolution{}, fmt.Errorf("agentdir: manifest plugin path is required")
		}
		pluginPath := plugin.Path
		if !filepath.IsAbs(pluginPath) {
			pluginPath = filepath.Join(dir, pluginPath)
		}
		bundle, err := LoadDir(pluginPath)
		if err != nil {
			return Resolution{}, fmt.Errorf("load manifest plugin %s: %w", pluginPath, err)
		}
		if err := out.Bundle.Append(bundle); err != nil {
			return Resolution{}, err
		}
		out.Sources = append(out.Sources, pluginPath)
	}
	return out, nil
}

func readManifest(dir string) (string, AppManifest, bool, error) {
	for _, name := range manifestNames {
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return "", AppManifest{}, false, err
		}
		var manifest AppManifest
		if err := json.Unmarshal(data, &manifest); err != nil {
			return "", AppManifest{}, false, fmt.Errorf("parse %s: %w", path, err)
		}
		return path, manifest, true, nil
	}
	return "", AppManifest{}, false, nil
}

func osDirExists(dir string) (bool, error) {
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return info.IsDir(), nil
}

func (b *Bundle) Append(other Bundle) error {
	for _, spec := range other.AgentSpecs {
		for _, existing := range b.AgentSpecs {
			if existing.Name == spec.Name {
				return fmt.Errorf("agentdir: duplicate agent %q", spec.Name)
			}
		}
		b.AgentSpecs = append(b.AgentSpecs, spec)
	}
	for _, cmd := range other.Commands {
		for _, existing := range b.Commands {
			if existing.Spec().Name == cmd.Spec().Name {
				return fmt.Errorf("agentdir: duplicate command %q", cmd.Spec().Name)
			}
		}
		b.Commands = append(b.Commands, cmd)
	}
	b.SkillSources = append(b.SkillSources, other.SkillSources...)
	return nil
}

func SourceExists(fsys fs.FS, dir string) bool {
	ok, _ := dirExists(fsys, dir)
	return ok
}
