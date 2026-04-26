package agentdir

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/codewandler/agentsdk/agent"
	"github.com/stretchr/testify/require"
)

func TestLoadFSLoadsAgentsCommandsAndSkills(t *testing.T) {
	fsys := fstest.MapFS{
		".agents/agents/coder.md": {
			Data: []byte(`---
description: Coder agent
model: test/model
max-steps: 12
tools: [bash, file_read]
skills: [coder]
commands: [review]
---
You are a coder.`),
		},
		".agents/commands/review.md": {
			Data: []byte("---\ndescription: Review\n---\nReview {{.Query}}"),
		},
		".agents/skills/coder/SKILL.md": {
			Data: []byte("---\nname: coder\ndescription: Coder skill\n---\n# Coder"),
		},
	}

	bundle, err := LoadFS(fsys, ".")
	require.NoError(t, err)
	require.Len(t, bundle.AgentSpecs, 1)
	require.Equal(t, "coder", bundle.AgentSpecs[0].Name)
	require.Equal(t, "Coder agent", bundle.AgentSpecs[0].Description)
	require.Equal(t, "test/model", bundle.AgentSpecs[0].Inference.Model)
	require.Equal(t, 12, bundle.AgentSpecs[0].MaxSteps)
	require.Equal(t, []string{"bash", "file_read"}, bundle.AgentSpecs[0].Tools)
	require.Equal(t, []string{"coder"}, bundle.AgentSpecs[0].Skills)
	require.Equal(t, []string{"review"}, bundle.AgentSpecs[0].Commands)
	require.Contains(t, bundle.AgentSpecs[0].System, "You are a coder.")
	require.Len(t, bundle.Commands, 1)
	require.Equal(t, "review", bundle.Commands[0].Spec().Name)
	require.Len(t, bundle.SkillSources, 1)
	require.Equal(t, ".agents/skills", bundle.SkillSources[0].Root)
}

func TestResolveDirPrefersManifest(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "app.manifest.json"), `{"default_agent":"main","plugins":[{"path":"plugin"}]}`)
	writeFile(t, filepath.Join(dir, ".claude", "agents", "ignored.md"), "---\nname: ignored\n---\nignored")
	writeFile(t, filepath.Join(dir, "plugin", "agents", "main.md"), "---\nname: main\n---\nmain")

	resolved, err := ResolveDir(dir)
	require.NoError(t, err)
	require.Equal(t, "main", resolved.DefaultAgent)
	require.Len(t, resolved.Bundle.AgentSpecs, 1)
	require.Equal(t, "main", resolved.Bundle.AgentSpecs[0].Name)
}

func TestResolveDirProbesClaudeAndAgentsBeforeRoot(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".claude", "agents", "main.md"), "---\nname: main\n---\nmain")
	writeFile(t, filepath.Join(dir, ".agents", "agents", "reviewer.md"), "---\nname: reviewer\n---\nreviewer")
	writeFile(t, filepath.Join(dir, "agents", "ignored.md"), "---\nname: ignored\n---\nignored")

	resolved, err := ResolveDir(dir)
	require.NoError(t, err)
	var names []string
	for _, spec := range resolved.Bundle.AgentSpecs {
		names = append(names, spec.Name)
	}
	require.Equal(t, []string{"main", "reviewer"}, names)
}

func TestResolveDirFallsBackToPluginRoot(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "agents", "main.md"), "---\nname: main\n---\nmain")

	resolved, err := ResolveDir(dir)
	require.NoError(t, err)
	require.Len(t, resolved.Bundle.AgentSpecs, 1)
	require.Equal(t, "main", resolved.Bundle.AgentSpecs[0].Name)
}

func TestResolveFSLoadsEmbeddedPluginRoot(t *testing.T) {
	fsys := fstest.MapFS{
		".agents/agents/coder.md": {Data: []byte("---\nname: coder\n---\ncoder")},
	}

	resolved, err := ResolveFS(fsys, ".agents")
	require.NoError(t, err)
	require.Equal(t, []string{".agents"}, resolved.Sources)
	require.Len(t, resolved.Bundle.AgentSpecs, 1)
	require.Equal(t, "coder", resolved.Bundle.AgentSpecs[0].Name)
}

func TestAgentFrontmatterSkillSourcesAreLoadedOnSpec(t *testing.T) {
	fsys := fstest.MapFS{
		".agents/agents/coder.md": {Data: []byte(`---
name: coder
skills: [coder-extra]
skill-sources: [../extra-skills]
---
coder`)},
		".agents/extra-skills/coder/SKILL.md": {Data: []byte("---\nname: coder-extra\ndescription: Extra\n---\n# Extra")},
	}

	bundle, err := LoadFS(fsys, ".")
	require.NoError(t, err)
	require.Len(t, bundle.AgentSpecs, 1)
	require.Len(t, bundle.AgentSpecs[0].SkillSources, 1)
	require.Equal(t, ".agents/extra-skills", bundle.AgentSpecs[0].SkillSources[0].Root)
}

func TestResolveDefaultAgent(t *testing.T) {
	name, err := ResolveDefaultAgent([]string{"reviewer", "main"}, "", "")
	require.NoError(t, err)
	require.Equal(t, "main", name)

	name, err = ResolveDefaultAgent([]string{"reviewer", "helper"}, "helper", "")
	require.NoError(t, err)
	require.Equal(t, "helper", name)

	_, err = ResolveDefaultAgent([]string{"reviewer", "helper"}, "", "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "--agent")
}

func TestResolutionHelpers(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "app.manifest.json"), `{"default_agent":"main","plugins":[{"path":"plugin"}]}`)
	writeFile(t, filepath.Join(dir, "plugin", "agents", "main.md"), "---\nname: main\n---\nmain")

	resolved, err := ResolveDir(dir)
	require.NoError(t, err)
	require.Equal(t, []string{"main"}, resolved.AgentNames())

	name, err := resolved.ResolveDefaultAgent("")
	require.NoError(t, err)
	require.Equal(t, "main", name)

	err = resolved.UpdateAgentSpec("main", func(spec *agent.Spec) {
		spec.MaxSteps = 42
	})
	require.NoError(t, err)
	require.Equal(t, 42, resolved.Bundle.AgentSpecs[0].MaxSteps)

	err = resolved.UpdateAgentSpec("missing", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "available agents")
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}
