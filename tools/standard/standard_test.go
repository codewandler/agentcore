package standard

import (
	"testing"

	"github.com/codewandler/agentsdk/tool"
	"github.com/stretchr/testify/require"
)

func TestToolsIncludesBaseAndOptionals(t *testing.T) {
	tools := Tools(Options{
		IncludeGit:            true,
		IncludeTodo:           true,
		IncludeToolManagement: true,
		IncludeTurnDone:       true,
	})

	names := map[string]bool{}
	for _, t := range tools {
		names[t.Name()] = true
	}

	for _, name := range []string{
		"bash",
		"file_read",
		"web_fetch",
		"git_status",
		"todo",
		"tools_list",
		"turn_done",
	} {
		require.True(t, names[name], "missing %s", name)
	}
}

func TestDefaultToolsIncludesToolManagement(t *testing.T) {
	tools := DefaultTools()

	names := map[string]bool{}
	for _, t := range tools {
		names[t.Name()] = true
	}

	require.True(t, names["bash"])
	require.True(t, names["file_read"])
	require.True(t, names["web_fetch"])
	require.True(t, names["tools_list"])
}

func TestCatalogToolsIncludesOptionalStandardTools(t *testing.T) {
	tools := CatalogTools()

	names := map[string]bool{}
	for _, t := range tools {
		names[t.Name()] = true
	}

	for _, name := range []string{
		"git_status",
		"git_diff",
		"notify_send",
		"todo",
		"turn_done",
		"web_search",
	} {
		require.True(t, names[name], "missing %s", name)
	}
}

func TestDefaultToolsetOwnsActivationState(t *testing.T) {
	toolset := DefaultToolset()

	require.NotNil(t, toolset.Activation())
	require.NotEmpty(t, toolset.Tools())
	require.Equal(t, len(toolset.Tools()), len(toolset.ActiveTools()))

	deactivated := toolset.Activation().Deactivate("file_*")
	require.NotEmpty(t, deactivated)

	activeNames := map[string]bool{}
	for _, t := range toolset.ActiveTools() {
		activeNames[t.Name()] = true
	}
	require.False(t, activeNames["file_read"])
	require.True(t, activeNames["bash"])
}

func TestNewToolsetFromToolsUsesExplicitTools(t *testing.T) {
	custom := tool.New("custom", "test", func(ctx tool.Ctx, p struct{}) (tool.Result, error) {
		return tool.Text("ok"), nil
	})

	toolset := NewToolsetFromTools(custom)

	require.Len(t, toolset.Tools(), 1)
	require.Equal(t, "custom", toolset.Tools()[0].Name())
	require.Equal(t, []string{"custom"}, toolNames(toolset.ActiveTools()))
}

func toolNames(tools []tool.Tool) []string {
	out := make([]string, len(tools))
	for i, t := range tools {
		out[i] = t.Name()
	}
	return out
}
