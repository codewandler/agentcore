package activation

import (
	"testing"

	"github.com/codewandler/agentsdk/tool"
	"github.com/stretchr/testify/require"
)

func TestManagerActivateDeactivatePatterns(t *testing.T) {
	fileRead := testTool("file_read")
	fileWrite := testTool("file_write")
	bash := testTool("bash")
	m := New(fileRead, fileWrite, bash)

	require.ElementsMatch(t, []string{"file_read", "file_write", "bash"}, names(m.ActiveTools()))

	deactivated := m.Deactivate("file_*")
	require.ElementsMatch(t, []string{"file_read", "file_write"}, deactivated)
	require.ElementsMatch(t, []string{"bash"}, names(m.ActiveTools()))

	activated := m.Activate("file_read")
	require.Equal(t, []string{"file_read"}, activated)
	require.ElementsMatch(t, []string{"file_read", "bash"}, names(m.ActiveTools()))
}

func TestManagerRegisterDeduplicates(t *testing.T) {
	m := New(testTool("one"))
	require.NoError(t, m.Register(testTool("one"), testTool("two")))

	require.ElementsMatch(t, []string{"one", "two"}, names(m.AllTools()))
	require.ElementsMatch(t, []string{"one", "two"}, names(m.ActiveTools()))
}

func names(tools []tool.Tool) []string {
	out := make([]string, len(tools))
	for i, t := range tools {
		out[i] = t.Name()
	}
	return out
}

func testTool(name string) tool.Tool {
	return tool.New(name, "test tool", func(ctx tool.Ctx, p struct{}) (tool.Result, error) {
		return tool.Text("ok"), nil
	})
}
