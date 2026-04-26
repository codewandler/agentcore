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
