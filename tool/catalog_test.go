package tool

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCatalogSelectSupportsNamePatterns(t *testing.T) {
	catalog, err := NewCatalog(
		testCatalogTool("file_read"),
		testCatalogTool("file_write"),
		testCatalogTool("bash"),
	)
	require.NoError(t, err)

	tools, err := catalog.Select([]string{"file_*", "bash"})
	require.NoError(t, err)
	require.Len(t, tools, 3)
	require.Equal(t, "file_read", tools[0].Name())
	require.Equal(t, "file_write", tools[1].Name())
	require.Equal(t, "bash", tools[2].Name())
}

func TestCatalogSelectDeduplicatesOverlappingPatterns(t *testing.T) {
	catalog, err := NewCatalog(testCatalogTool("file_read"), testCatalogTool("file_write"))
	require.NoError(t, err)

	tools, err := catalog.Select([]string{"file_*", "file_read"})
	require.NoError(t, err)
	require.Len(t, tools, 2)
	require.Equal(t, "file_read", tools[0].Name())
	require.Equal(t, "file_write", tools[1].Name())
}

func TestCatalogSelectPatternRequiresMatch(t *testing.T) {
	catalog, err := NewCatalog(testCatalogTool("bash"))
	require.NoError(t, err)

	_, err = catalog.Select([]string{"file_*"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "matched no tools")
}

func testCatalogTool(name string) Tool {
	return New(name, name, func(Ctx, struct{}) (Result, error) { return Text("ok"), nil })
}
