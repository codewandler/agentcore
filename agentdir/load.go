// Package agentdir loads filesystem-described agent resources.
package agentdir

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/codewandler/agentsdk/agent"
	"github.com/codewandler/agentsdk/command"
	cmdmarkdown "github.com/codewandler/agentsdk/command/markdown"
	md "github.com/codewandler/agentsdk/markdown"
	"github.com/codewandler/agentsdk/skill"
	"github.com/codewandler/llmadapter/unified"
)

// Bundle is a loaded set of agent resources.
type Bundle struct {
	AgentSpecs   []agent.Spec
	Commands     []command.Command
	SkillSources []skill.Source
}

type AgentFrontmatter struct {
	Name         string   `yaml:"name"`
	Description  string   `yaml:"description"`
	Model        string   `yaml:"model"`
	MaxTokens    int      `yaml:"max-tokens"`
	MaxSteps     int      `yaml:"max-steps"`
	Temperature  float64  `yaml:"temperature"`
	Thinking     string   `yaml:"thinking"`
	Effort       string   `yaml:"effort"`
	Tools        []string `yaml:"tools"`
	Skills       []string `yaml:"skills"`
	Commands     []string `yaml:"commands"`
	SkillSources []string `yaml:"skill-sources"`
}

// LoadDir loads resources from an OS directory.
func LoadDir(dir string) (Bundle, error) {
	return LoadFS(os.DirFS(dir), ".")
}

// LoadFS loads resources from fsys rooted at root.
func LoadFS(fsys fs.FS, root string) (Bundle, error) {
	var out Bundle
	root = clean(root)
	for _, dir := range []string{
		path.Join(root, ".claude", "commands"),
		path.Join(root, ".agents", "commands"),
		path.Join(root, "commands"),
	} {
		cmds, err := cmdmarkdown.LoadFS(fsys, dir)
		if err != nil {
			return Bundle{}, err
		}
		out.Commands = append(out.Commands, cmds...)
	}
	for _, dir := range []string{
		path.Join(root, ".claude", "agents"),
		path.Join(root, ".agents", "agents"),
		path.Join(root, "agents"),
	} {
		specs, err := loadAgentSpecs(fsys, dir)
		if err != nil {
			return Bundle{}, err
		}
		out.AgentSpecs = append(out.AgentSpecs, specs...)
	}
	for order, dir := range []string{
		path.Join(root, ".claude", "skills"),
		path.Join(root, ".agents", "skills"),
		path.Join(root, "skills"),
	} {
		if exists, err := dirExists(fsys, dir); err != nil {
			return Bundle{}, err
		} else if exists {
			out.SkillSources = append(out.SkillSources, skill.FSSource(dir, dir, fsys, dir, sourceKind(dir), order))
		}
	}
	return out, nil
}

func loadAgentSpecs(fsys fs.FS, dir string) ([]agent.Spec, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []agent.Spec
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		file := path.Join(dir, entry.Name())
		data, err := fs.ReadFile(fsys, file)
		if err != nil {
			return nil, fmt.Errorf("read agent spec %q: %w", file, err)
		}
		spec, fm, err := parseAgentSpec(entry.Name(), data)
		if err != nil {
			return nil, fmt.Errorf("parse agent spec %q: %w", file, err)
		}
		spec.SkillSources = append(spec.SkillSources, skillSourcesFromFrontmatter(fsys, path.Dir(file), file, fm.SkillSources)...)
		out = append(out, spec)
	}
	return out, nil
}

func ParseAgentSpec(name string, content []byte) (agent.Spec, error) {
	spec, _, err := parseAgentSpec(name, content)
	return spec, err
}

func parseAgentSpec(name string, content []byte) (agent.Spec, AgentFrontmatter, error) {
	meta, body, err := md.Parse(strings.NewReader(string(content)))
	if err != nil {
		return agent.Spec{}, AgentFrontmatter{}, err
	}
	fm, err := md.Bind[AgentFrontmatter](meta)
	if err != nil {
		return agent.Spec{}, AgentFrontmatter{}, err
	}
	if fm.Name == "" {
		fm.Name = strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
	}
	inference := agent.DefaultInferenceOptions()
	if fm.Model != "" {
		inference.Model = fm.Model
	}
	if fm.MaxTokens > 0 {
		inference.MaxTokens = fm.MaxTokens
	}
	if fm.Temperature != 0 {
		inference.Temperature = fm.Temperature
	}
	if fm.Thinking != "" {
		inference.Thinking = agent.ThinkingMode(fm.Thinking)
	}
	if fm.Effort != "" {
		inference.Effort = unified.ReasoningEffort(fm.Effort)
	}
	return agent.Spec{
		Name:        fm.Name,
		Description: fm.Description,
		System:      body,
		Inference:   inference,
		MaxSteps:    fm.MaxSteps,
		Tools:       append([]string(nil), fm.Tools...),
		Skills:      append([]string(nil), fm.Skills...),
		Commands:    append([]string(nil), fm.Commands...),
	}, fm, nil
}

func skillSourcesFromFrontmatter(fsys fs.FS, agentDir string, agentFile string, roots []string) []skill.Source {
	var sources []skill.Source
	for order, root := range roots {
		root = clean(root)
		if root == "." {
			continue
		}
		sourceRoot := root
		if !path.IsAbs(sourceRoot) {
			sourceRoot = path.Join(agentDir, sourceRoot)
		}
		sourceRoot = clean(sourceRoot)
		id := fmt.Sprintf("%s:skill-sources:%s", agentFile, root)
		sources = append(sources, skill.FSSource(id, sourceRoot, fsys, sourceRoot, sourceKind(sourceRoot), order))
	}
	return sources
}

func clean(root string) string {
	root = strings.TrimPrefix(filepath.ToSlash(root), "/")
	if root == "" || root == "." {
		return "."
	}
	return path.Clean(root)
}

func dirExists(fsys fs.FS, dir string) (bool, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return len(entries) >= 0, nil
}

func sourceKind(dir string) skill.SourceKind {
	switch {
	case strings.Contains(dir, ".claude"):
		return skill.SourceClaudeProject
	case strings.Contains(dir, ".agents"):
		return skill.SourceAgentsCompat
	default:
		return skill.SourcePluginRoot
	}
}
