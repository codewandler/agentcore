package skill

import (
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
)

func TestRepositoryPreservesInputOrderWhenSourceOrdersMatch(t *testing.T) {
	fsys := fstest.MapFS{
		"one/a/SKILL.md": {Data: []byte("---\nname: a\ndescription: A\n---\nA")},
		"two/b/SKILL.md": {Data: []byte("---\nname: b\ndescription: B\n---\nB")},
	}
	sources := []Source{
		FSSource("z-source", "one", fsys, "one", SourceEmbedded, 0),
		FSSource("a-source", "two", fsys, "two", SourceEmbedded, 0),
	}

	repo, err := NewRepository(sources, nil)
	require.NoError(t, err)

	got := repo.Sources()
	require.Equal(t, "z-source", got[0].ID)
	require.Equal(t, "a-source", got[1].ID)
}

func TestRepositoryDuplicateSkillFirstSourceWins(t *testing.T) {
	fsys := fstest.MapFS{
		"one/shared/SKILL.md": {Data: []byte("---\nname: shared\ndescription: First\n---\nFirst body")},
		"two/shared/SKILL.md": {Data: []byte("---\nname: shared\ndescription: Second\n---\nSecond body")},
	}
	sources := []Source{
		FSSource("one", "one", fsys, "one", SourceEmbedded, 0),
		FSSource("two", "two", fsys, "two", SourceEmbedded, 1),
	}

	repo, err := NewRepository(sources, []string{"shared"})
	require.NoError(t, err)

	loaded := repo.Loaded()
	require.Len(t, loaded, 1)
	require.Equal(t, "First", loaded[0].Description)
	require.Equal(t, "one", loaded[0].SourceID)
}
