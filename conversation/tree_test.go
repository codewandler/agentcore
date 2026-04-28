package conversation

import (
	"testing"

	"github.com/codewandler/llmadapter/unified"
	"github.com/stretchr/testify/require"
)

func userPayload(text string) MessageEvent {
	return MessageEvent{Message: unified.Message{Role: unified.RoleUser, Content: []unified.ContentPart{unified.TextPart{Text: text}}}}
}

func TestTreePathReturnsAllNodes(t *testing.T) {
	tree := NewTree()
	a, err := tree.Append(MainBranch, userPayload("a"))
	require.NoError(t, err)
	b, err := tree.Append(MainBranch, userPayload("b"))
	require.NoError(t, err)
	c, err := tree.Append(MainBranch, userPayload("c"))
	require.NoError(t, err)

	path, err := tree.Path(MainBranch)
	require.NoError(t, err)
	require.Len(t, path, 3)
	require.Equal(t, a, path[0].ID)
	require.Equal(t, b, path[1].ID)
	require.Equal(t, c, path[2].ID)
}

func TestTreePathStopsAtFloor(t *testing.T) {
	tree := NewTree()
	a, err := tree.Append(MainBranch, userPayload("a"))
	require.NoError(t, err)
	b, err := tree.Append(MainBranch, userPayload("b"))
	require.NoError(t, err)
	c, err := tree.Append(MainBranch, userPayload("c"))
	require.NoError(t, err)

	// Without floor: full path.
	path, err := tree.Path(MainBranch)
	require.NoError(t, err)
	require.Len(t, path, 3)

	// Set floor to b: path starts at b.
	tree.SetFloor(MainBranch, b)
	path, err = tree.Path(MainBranch)
	require.NoError(t, err)
	require.Len(t, path, 2)
	require.Equal(t, b, path[0].ID)
	require.Equal(t, c, path[1].ID)

	// Clear floor: full path again.
	tree.SetFloor(MainBranch, "")
	path, err = tree.Path(MainBranch)
	require.NoError(t, err)
	require.Len(t, path, 3)
	_ = a // suppress unused
}

func TestTreeFloorDoesNotAffectOtherBranches(t *testing.T) {
	tree := NewTree()
	_, err := tree.Append(MainBranch, userPayload("a"))
	require.NoError(t, err)
	b, err := tree.Append(MainBranch, userPayload("b"))
	require.NoError(t, err)
	_, err = tree.Append(MainBranch, userPayload("c"))
	require.NoError(t, err)

	require.NoError(t, tree.Fork(MainBranch, "other"))
	tree.SetFloor(MainBranch, b)

	// Main branch: 2 nodes (b, c).
	mainPath, err := tree.Path(MainBranch)
	require.NoError(t, err)
	require.Len(t, mainPath, 2)

	// Other branch: still 3 nodes (forked before floor was set).
	otherPath, err := tree.Path("other")
	require.NoError(t, err)
	require.Len(t, otherPath, 3)
}

func TestTreeFloorAtHead(t *testing.T) {
	tree := NewTree()
	_, err := tree.Append(MainBranch, userPayload("a"))
	require.NoError(t, err)
	_, err = tree.Append(MainBranch, userPayload("b"))
	require.NoError(t, err)
	c, err := tree.Append(MainBranch, userPayload("c"))
	require.NoError(t, err)

	// Floor at head: path has only the head node.
	tree.SetFloor(MainBranch, c)
	path, err := tree.Path(MainBranch)
	require.NoError(t, err)
	require.Len(t, path, 1)
	require.Equal(t, c, path[0].ID)
}

func TestTreeFloorDefaultBranch(t *testing.T) {
	tree := NewTree()
	_, err := tree.Append("", userPayload("a"))
	require.NoError(t, err)
	b, err := tree.Append("", userPayload("b"))
	require.NoError(t, err)

	// SetFloor with empty branch defaults to MainBranch.
	tree.SetFloor("", b)
	floor, ok := tree.Floor("")
	require.True(t, ok)
	require.Equal(t, b, floor)

	path, err := tree.Path("")
	require.NoError(t, err)
	require.Len(t, path, 1)
	require.Equal(t, b, path[0].ID)
}

func TestTreeFloorNotSetReturnsNotOK(t *testing.T) {
	tree := NewTree()
	_, ok := tree.Floor(MainBranch)
	require.False(t, ok)
}
