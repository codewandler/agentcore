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

func TestCatalog_ApplyAll(t *testing.T) {
	catalog, err := NewCatalog(
		testCatalogTool("file_read"),
		testCatalogTool("file_write"),
		testCatalogTool("bash"),
	)
	require.NoError(t, err)

	// Apply a middleware that renames all tools.
	catalog.ApplyAll(HooksMiddleware(&renameHooks{newName: "wrapped"}))

	// All tools should now have the wrapped name.
	for _, tl := range catalog.All() {
		require.Equal(t, "wrapped", tl.Name())
	}

	// But catalog keys are preserved — Select by original name still works.
	tools, err := catalog.Select([]string{"file_read"})
	require.NoError(t, err)
	require.Len(t, tools, 1)
	require.Equal(t, "wrapped", tools[0].Name())
}

func TestCatalog_ApplyTo_ExactName(t *testing.T) {
	catalog, err := NewCatalog(
		testCatalogTool("file_read"),
		testCatalogTool("file_write"),
		testCatalogTool("bash"),
	)
	require.NoError(t, err)

	matched := catalog.ApplyTo("bash", HooksMiddleware(&renameHooks{newName: "safe_bash"}))
	require.Equal(t, 1, matched)

	// Only bash should be renamed.
	tools := catalog.All()
	require.Equal(t, "file_read", tools[0].Name())
	require.Equal(t, "file_write", tools[1].Name())
	require.Equal(t, "safe_bash", tools[2].Name())
}

func TestCatalog_ApplyTo_GlobPattern(t *testing.T) {
	catalog, err := NewCatalog(
		testCatalogTool("file_read"),
		testCatalogTool("file_write"),
		testCatalogTool("bash"),
	)
	require.NoError(t, err)

	matched := catalog.ApplyTo("file_*", HooksMiddleware(&renameHooks{newName: "fs_tool"}))
	require.Equal(t, 2, matched)

	tools := catalog.All()
	require.Equal(t, "fs_tool", tools[0].Name())
	require.Equal(t, "fs_tool", tools[1].Name())
	require.Equal(t, "bash", tools[2].Name())
}

func TestCatalog_ApplyTo_NoMatch(t *testing.T) {
	catalog, err := NewCatalog(testCatalogTool("bash"))
	require.NoError(t, err)

	matched := catalog.ApplyTo("file_*", HooksMiddleware(&renameHooks{newName: "x"}))
	require.Equal(t, 0, matched)
}

func TestCatalog_ApplyAll_NilCatalog(t *testing.T) {
	var c *Catalog
	c.ApplyAll(HooksMiddleware(&renameHooks{newName: "x"})) // should not panic
}
