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

func TestRepositoryDiscoversSkillReferences(t *testing.T) {
	fsys := fstest.MapFS{
		"skills/coder/SKILL.md":            {Data: []byte("---\nname: coder\ndescription: Coding help\n---\n# Coder")},
		"skills/coder/references/alpha.md": {Data: []byte("---\ntriggers: [plan, review]\n---\nAlpha body")},
		"skills/coder/references/zeta.md":  {Data: []byte("Zeta body")},
	}

	repo, err := NewRepository([]Source{FSSource("skills", "skills", fsys, "skills", SourceEmbedded, 0)}, nil)
	require.NoError(t, err)

	refs := repo.ListReferences("coder")
	require.Len(t, refs, 2)
	require.Equal(t, "references/alpha.md", refs[0].Path)
	require.Equal(t, []string{"plan", "review"}, refs[0].Metadata.AllTriggers())
	require.Equal(t, "references/zeta.md", refs[1].Path)
	require.Empty(t, refs[1].Metadata.AllTriggers())
}

func TestRepositoryGetReferenceUsesExactRelativePath(t *testing.T) {
	fsys := fstest.MapFS{
		"skills/coder/SKILL.md":             {Data: []byte("---\nname: coder\ndescription: Coding help\n---\n# Coder")},
		"skills/coder/references/review.md": {Data: []byte("---\ntrigger: review\n---\nReview body")},
	}

	repo, err := NewRepository([]Source{FSSource("skills", "skills", fsys, "skills", SourceEmbedded, 0)}, nil)
	require.NoError(t, err)

	ref, ok := repo.GetReference("coder", "references/review.md")
	require.True(t, ok)
	require.Equal(t, "references/review.md", ref.Path)

	_, ok = repo.GetReference("coder", "review.md")
	require.False(t, ok)
}

func TestValidReferencePath(t *testing.T) {
	tests := []struct {
		name string
		path string
		want bool
	}{
		{name: "valid", path: "references/review.md", want: true},
		{name: "nested valid", path: "references/golang/errors.md", want: true},
		{name: "empty", path: "", want: false},
		{name: "absolute", path: "/references/review.md", want: false},
		{name: "outside references", path: "notes/review.md", want: false},
		{name: "traversal", path: "references/../review.md", want: false},
		{name: "parent", path: "../references/review.md", want: false},
		{name: "skill root file", path: "SKILL.md", want: false},
		{name: "skill file under references", path: "references/SKILL.md", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, validReferencePath(tc.path))
		})
	}
}

func TestActivationStateBaselineSkillsActiveAtStartup(t *testing.T) {
	fsys := fstest.MapFS{
		"skills/coder/SKILL.md": {Data: []byte("---\nname: coder\ndescription: Coding help\n---\n# Coder")},
	}
	repo, err := NewRepository([]Source{FSSource("skills", "skills", fsys, "skills", SourceEmbedded, 0)}, nil)
	require.NoError(t, err)

	state, err := NewActivationState(repo, []string{"coder"})
	require.NoError(t, err)
	require.Equal(t, StatusBase, state.Status("coder"))
	require.True(t, state.IsActive("coder"))
	require.Equal(t, []string{"coder"}, state.ActiveSkillNames())
}

func TestActivationStateActivateSkillIsIdempotent(t *testing.T) {
	fsys := fstest.MapFS{
		"skills/coder/SKILL.md": {Data: []byte("---\nname: coder\ndescription: Coding help\n---\n# Coder")},
	}
	repo, err := NewRepository([]Source{FSSource("skills", "skills", fsys, "skills", SourceEmbedded, 0)}, nil)
	require.NoError(t, err)

	state, err := NewActivationState(repo, nil)
	require.NoError(t, err)

	status, err := state.ActivateSkill("coder")
	require.NoError(t, err)
	require.Equal(t, StatusDynamic, status)

	status, err = state.ActivateSkill("coder")
	require.NoError(t, err)
	require.Equal(t, StatusDynamic, status)
	require.Equal(t, []string{"coder"}, state.ActiveSkillNames())
}

func TestActivationStateActivateReferencesRequiresActiveSkill(t *testing.T) {
	fsys := fstest.MapFS{
		"skills/coder/SKILL.md":             {Data: []byte("---\nname: coder\ndescription: Coding help\n---\n# Coder")},
		"skills/coder/references/review.md": {Data: []byte("---\ntrigger: review\n---\nReview body")},
	}
	repo, err := NewRepository([]Source{FSSource("skills", "skills", fsys, "skills", SourceEmbedded, 0)}, nil)
	require.NoError(t, err)

	state, err := NewActivationState(repo, nil)
	require.NoError(t, err)

	_, err = state.ActivateReferences("coder", []string{"references/review.md"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "require the skill to be active first")
}

func TestActivationStateActivateReferencesRejectsInvalidPath(t *testing.T) {
	fsys := fstest.MapFS{
		"skills/coder/SKILL.md":             {Data: []byte("---\nname: coder\ndescription: Coding help\n---\n# Coder")},
		"skills/coder/references/review.md": {Data: []byte("---\ntrigger: review\n---\nReview body")},
	}
	repo, err := NewRepository([]Source{FSSource("skills", "skills", fsys, "skills", SourceEmbedded, 0)}, nil)
	require.NoError(t, err)

	state, err := NewActivationState(repo, []string{"coder"})
	require.NoError(t, err)

	_, err = state.ActivateReferences("coder", []string{"../review.md"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid reference path")
}

func TestActivationStateMaterializeIncludesActivatedReferences(t *testing.T) {
	fsys := fstest.MapFS{
		"skills/coder/SKILL.md":             {Data: []byte("---\nname: coder\ndescription: Coding help\n---\n# Coder")},
		"skills/coder/references/review.md": {Data: []byte("---\ntrigger: review\n---\nReview body")},
	}
	repo, err := NewRepository([]Source{FSSource("skills", "skills", fsys, "skills", SourceEmbedded, 0)}, nil)
	require.NoError(t, err)

	state, err := NewActivationState(repo, []string{"coder"})
	require.NoError(t, err)

	_, err = state.ActivateReferences("coder", []string{"references/review.md"})
	require.NoError(t, err)

	materialized := state.Materialize()
	require.Contains(t, materialized, "## coder")
	require.Contains(t, materialized, "### references/review.md")
	require.Contains(t, materialized, "Review body")
}
