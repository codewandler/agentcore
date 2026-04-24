package standard

import (
	"testing"

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
