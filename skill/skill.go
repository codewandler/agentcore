// Package skill defines Agent Skills-compatible filesystem resources.
package skill

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"sort"
	"strings"

	md "github.com/codewandler/agentsdk/markdown"
)

// ErrNotLoaded is returned by Unload when the requested skill is not currently active.
var ErrNotLoaded = errors.New("skill not loaded")

// SourceKind describes the resource layout represented by a source.
type SourceKind string

const (
	SourceClaudeProject SourceKind = "claude_project"
	SourceClaudeUser    SourceKind = "claude_user"
	SourceAgentsCompat  SourceKind = "agents_compat"
	SourcePluginRoot    SourceKind = "plugin_root"
	SourceEmbedded      SourceKind = "embedded"
)

// Source points at a directory whose immediate children are skill directories.
type Source struct {
	ID    string
	Label string
	Kind  SourceKind
	FS    fs.FS
	Root  string
	Order int
}

// DirSource returns an OS directory skill source.
func DirSource(id, label, dir string, kind SourceKind, order int) Source {
	if label == "" {
		label = dir
	}
	return Source{ID: id, Label: label, Kind: kind, FS: os.DirFS(dir), Root: ".", Order: order}
}

// FSSource returns an embedded or virtual filesystem skill source.
func FSSource(id, label string, fsys fs.FS, root string, kind SourceKind, order int) Source {
	if root == "" {
		root = "."
	}
	return Source{ID: id, Label: label, Kind: kind, FS: fsys, Root: clean(root), Order: order}
}

// Skill is a discovered Agent Skill directory.
type Skill struct {
	Name        string
	Description string
	Metadata    SkillMetadata
	SourceID    string
	SourceLabel string
	Dir         string
	Body        string
}

// Repository is the resolved skill catalog and loaded skill set for one agent.
type Repository struct {
	sources []Source
	skills  map[string]Skill
	loaded  map[string]Skill
	order   []string
}

// NewRepository scans sources and loads default skill names.
func NewRepository(sources []Source, defaultNames []string) (*Repository, error) {
	repo := &Repository{
		sources: append([]Source(nil), sources...),
		skills:  map[string]Skill{},
		loaded:  map[string]Skill{},
	}
	if err := repo.scan(); err != nil {
		return nil, err
	}
	for _, name := range defaultNames {
		if err := repo.Load(name); err != nil {
			if len(repo.sources) == 0 {
				continue
			}
			return nil, err
		}
	}
	return repo, nil
}

// Sources returns the configured source list in deterministic scan order.
func (r *Repository) Sources() []Source {
	if r == nil {
		return nil
	}
	out := append([]Source(nil), r.sources...)
	sortSources(out)
	return out
}

// List returns all discovered skills ordered by name.
func (r *Repository) List() []Skill {
	if r == nil {
		return nil
	}
	names := make([]string, 0, len(r.skills))
	for name := range r.skills {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]Skill, 0, len(names))
	for _, name := range names {
		out = append(out, r.skills[name])
	}
	return out
}

// Get returns a discovered skill by name.
func (r *Repository) Get(name string) (Skill, bool) {
	if r == nil {
		return Skill{}, false
	}
	s, ok := r.skills[strings.TrimSpace(name)]
	return s, ok
}

// Load marks a discovered skill as loaded for system-context materialization.
func (r *Repository) Load(name string) error {
	if r == nil {
		return fmt.Errorf("skill: repository is nil")
	}
	name = strings.TrimSpace(name)
	if _, ok := r.loaded[name]; ok {
		return nil
	}
	s, ok := r.skills[name]
	if !ok {
		return fmt.Errorf("skill: %q not found", name)
	}
	r.loaded[name] = s
	r.order = append(r.order, name)
	return nil
}

// Loaded returns loaded skills in load order.
func (r *Repository) Loaded() []Skill {
	if r == nil {
		return nil
	}
	out := make([]Skill, 0, len(r.order))
	for _, name := range r.order {
		if s, ok := r.loaded[name]; ok {
			out = append(out, s)
		}
	}
	return out
}

// LoadedNames returns loaded skill names in load order.
func (r *Repository) LoadedNames() []string {
	if r == nil {
		return nil
	}
	return append([]string(nil), r.order...)
}

// Materialize returns deterministic system-context text for loaded skills.
func (r *Repository) Materialize() string {
	if r == nil || len(r.order) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Loaded skills:\n")
	for _, s := range r.Loaded() {
		b.WriteString("\n## ")
		b.WriteString(s.Name)
		if s.Description != "" {
			b.WriteString("\n")
			b.WriteString(s.Description)
		}
		if strings.TrimSpace(s.Body) != "" {
			b.WriteString("\n\n")
			b.WriteString(strings.TrimSpace(s.Body))
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func (r *Repository) scan() error {
	sortSources(r.sources)
	for _, source := range r.sources {
		skills, err := loadSource(source)
		if err != nil {
			return err
		}
		for _, s := range skills {
			if _, ok := r.skills[s.Name]; ok {
				continue
			}
			r.skills[s.Name] = s
		}
	}
	return nil
}

func loadSource(source Source) ([]Skill, error) {
	if source.FS == nil {
		return nil, nil
	}
	root := clean(source.Root)
	entries, err := fs.ReadDir(source.FS, root)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) || os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("skill: read source %s: %w", sourceLabel(source), err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	var out []Skill
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := path.Join(root, entry.Name())
		data, err := fs.ReadFile(source.FS, path.Join(dir, "SKILL.md"))
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) || os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("skill: read %s/SKILL.md: %w", dir, err)
		}
		meta, body, err := md.Parse(strings.NewReader(string(data)))
		if err != nil {
			return nil, fmt.Errorf("skill: parse %s/SKILL.md: %w", dir, err)
		}
		fm, err := md.Bind[SkillMetadata](meta)
		if err != nil {
			return nil, fmt.Errorf("skill: parse metadata %s/SKILL.md: %w", dir, err)
		}
		if fm.Name == "" {
			return nil, fmt.Errorf("skill: %s/SKILL.md missing required name", dir)
		}
		if fm.Description == "" {
			return nil, fmt.Errorf("skill: %s/SKILL.md missing required description", dir)
		}
		out = append(out, Skill{
			Name:        fm.Name,
			Description: fm.Description,
			Metadata:    fm,
			SourceID:    source.ID,
			SourceLabel: sourceLabel(source),
			Dir:         dir,
			Body:        strings.TrimSpace(body),
		})
	}
	return out, nil
}

func sortSources(sources []Source) {
	sort.SliceStable(sources, func(i, j int) bool {
		return sources[i].Order < sources[j].Order
	})
}

func sourceLabel(source Source) string {
	if source.Label != "" {
		return source.Label
	}
	if source.ID != "" {
		return source.ID
	}
	return string(source.Kind)
}

func clean(root string) string {
	root = strings.TrimPrefix(path.Clean(strings.TrimSpace(root)), "/")
	if root == "" || root == "." {
		return "."
	}
	return root
}

// RegistryKey is the Extra() key under which skill tools look up the Repository.
const RegistryKey = "flai.skill_registry"
