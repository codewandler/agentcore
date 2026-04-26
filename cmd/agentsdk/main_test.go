package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRootCommandRegistersRun(t *testing.T) {
	cmd := rootCmd()
	run, _, err := cmd.Find([]string{"run"})
	require.NoError(t, err)
	require.NotNil(t, run)
	require.Equal(t, "run", run.Name())
	discover, _, err := cmd.Find([]string{"discover"})
	require.NoError(t, err)
	require.NotNil(t, discover)
	require.Equal(t, "discover", discover.Name())
}

func TestRunRejectsUnknownFlag(t *testing.T) {
	err := run([]string{"run", "./agent", "--bad"})
	require.Error(t, err)
}

func TestDiscoverPrintsResourcesAndDisabledSuggestions(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, ".agents", "agents", "main.md"), "---\nname: main\ndescription: Main agent\n---\nmain")
	writeTestFile(t, filepath.Join(dir, ".agents", "commands", "review.md"), "---\ndescription: Review command\n---\nreview")
	writeTestFile(t, filepath.Join(dir, ".agents", "skills", "go", "SKILL.md"), "---\nname: go\ndescription: Go skill\n---\n# Go")
	writeTestFile(t, filepath.Join(dir, "Makefile"), "test:\n\tgo test ./...\n")

	cmd := rootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"discover", "--local", dir})
	require.NoError(t, cmd.Execute())

	text := out.String()
	require.Contains(t, text, "Agents:")
	require.Contains(t, text, "main")
	require.Contains(t, text, "Main agent")
	require.Contains(t, text, "Commands:")
	require.Contains(t, text, "/review")
	require.Contains(t, text, "Review command")
	require.Contains(t, text, "Skills:")
	require.Contains(t, text, "go")
	require.Contains(t, text, "Go skill")
	require.Contains(t, text, "Disabled suggestions:")
	require.Contains(t, text, "Makefile")
}

func TestDiscoverPrintsFirstWinsDiagnostics(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, ".agents", "agents", "reviewer.md"), "---\nname: reviewer\n---\nfirst")
	writeTestFile(t, filepath.Join(dir, ".claude", "agents", "reviewer.md"), "---\nname: reviewer\n---\nsecond")

	cmd := rootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"discover", "--local", dir})
	require.NoError(t, cmd.Execute())

	text := out.String()
	require.Equal(t, 1, bytes.Count(out.Bytes(), []byte("  reviewer  ")))
	require.Contains(t, text, "warning")
	require.Contains(t, text, "already registered")
}

func TestDiscoverFormatsMultilineDescriptions(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, ".agents", "agents", "main.md"), "---\nname: main\ndescription: \"Line one\\nLine two\"\n---\nmain")

	cmd := rootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"discover", "--local", dir})
	require.NoError(t, cmd.Execute())

	require.Contains(t, out.String(), "Line one Line two")
	require.NotContains(t, out.String(), `Line one\nLine two`)
}

func TestDiscoverTruncatesLongDescriptions(t *testing.T) {
	dir := t.TempDir()
	long := strings.Repeat("word ", 80)
	writeTestFile(t, filepath.Join(dir, ".agents", "agents", "main.md"), "---\nname: main\ndescription: "+long+"\n---\nmain")

	cmd := rootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"discover", "--local", dir})
	require.NoError(t, cmd.Execute())

	require.Contains(t, out.String(), "...")
	require.NotContains(t, out.String(), long)
}

func writeTestFile(t *testing.T, path string, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}
