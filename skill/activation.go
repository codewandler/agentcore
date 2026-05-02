package skill

import (
	"fmt"
	"sort"
	"strings"
)

// ContextKey is the tool context Extra key used to pass mutable skill activation
// state to skill tool projections.
const ContextKey = "agentsdk.skill_activation_state"

// Status reports how a skill is activated in the current session state.
type Status string

const (
	StatusInactive Status = "inactive"
	StatusBase     Status = "base"
	StatusDynamic  Status = "dynamic"
)

// ActivationState wraps an immutable Repository with mutable session-scoped
// activation state for skills and their references.
type ActivationState struct {
	repo             *Repository
	baseSkills       map[string]bool
	dynamicSkills    map[string]bool
	activeReferences map[string]map[string]bool
	diagnostics      []string
}

// NewActivationState builds a mutable activation wrapper for one discovered
// skill catalog. baseline skills must already exist in the repository.
func NewActivationState(repo *Repository, baseline []string) (*ActivationState, error) {
	if repo == nil {
		return nil, fmt.Errorf("skill: repository is nil")
	}
	state := &ActivationState{
		repo:             repo,
		baseSkills:       map[string]bool{},
		dynamicSkills:    map[string]bool{},
		activeReferences: map[string]map[string]bool{},
	}
	for _, name := range baseline {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, ok := repo.Get(name); !ok {
			return nil, fmt.Errorf("skill: %q not found", name)
		}
		state.baseSkills[name] = true
	}
	return state, nil
}

// Repository returns the immutable discovered skill catalog.
func (s *ActivationState) Repository() *Repository {
	if s == nil {
		return nil
	}
	return s.repo
}

// Diagnostics returns deterministic non-fatal activation diagnostics.
func (s *ActivationState) Diagnostics() []string {
	if s == nil {
		return nil
	}
	return append([]string(nil), s.diagnostics...)
}

func (s *ActivationState) AddDiagnostic(format string, args ...any) {
	if s == nil {
		return
	}
	s.diagnostics = append(s.diagnostics, fmt.Sprintf(format, args...))
}

// Status returns the activation status for a discovered skill.
func (s *ActivationState) Status(name string) Status {
	if s == nil {
		return StatusInactive
	}
	name = strings.TrimSpace(name)
	if s.baseSkills[name] {
		return StatusBase
	}
	if s.dynamicSkills[name] {
		return StatusDynamic
	}
	return StatusInactive
}

// IsActive reports whether a skill is active through either baseline or dynamic state.
func (s *ActivationState) IsActive(name string) bool {
	status := s.Status(name)
	return status == StatusBase || status == StatusDynamic
}

// ActivateSkill activates one discovered skill dynamically. Baseline skills are
// treated as already active and do not change status.
func (s *ActivationState) ActivateSkill(name string) (Status, error) {
	if s == nil || s.repo == nil {
		return StatusInactive, fmt.Errorf("skill: activation state is nil")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return StatusInactive, fmt.Errorf("skill: skill name is required")
	}
	if _, ok := s.repo.Get(name); !ok {
		return StatusInactive, fmt.Errorf("skill: %q not found", name)
	}
	if s.baseSkills[name] {
		return StatusBase, nil
	}
	if s.dynamicSkills[name] {
		return StatusDynamic, nil
	}
	s.dynamicSkills[name] = true
	return StatusDynamic, nil
}

// ActivateReferences activates exact reference paths for an already-active skill.
func (s *ActivationState) ActivateReferences(skillName string, refPaths []string) ([]string, error) {
	if s == nil || s.repo == nil {
		return nil, fmt.Errorf("skill: activation state is nil")
	}
	skillName = strings.TrimSpace(skillName)
	if !s.IsActive(skillName) {
		return nil, fmt.Errorf("skill: references for %q require the skill to be active first", skillName)
	}
	activated := make([]string, 0, len(refPaths))
	seen := map[string]bool{}
	for _, raw := range refPaths {
		refPath := strings.TrimSpace(raw)
		if seen[refPath] {
			continue
		}
		seen[refPath] = true
		if !validReferencePath(refPath) {
			return nil, fmt.Errorf("skill: invalid reference path %q", refPath)
		}
		if _, ok := s.repo.GetReference(skillName, refPath); !ok {
			return nil, fmt.Errorf("skill: reference %q not found for skill %q", refPath, skillName)
		}
		if s.activeReferences[skillName] == nil {
			s.activeReferences[skillName] = map[string]bool{}
		}
		if s.activeReferences[skillName][refPath] {
			continue
		}
		s.activeReferences[skillName][refPath] = true
		activated = append(activated, refPath)
	}
	sort.Strings(activated)
	return activated, nil
}

// ActiveSkills returns active discovered skills in deterministic order.
func (s *ActivationState) ActiveSkills() []Skill {
	if s == nil || s.repo == nil {
		return nil
	}
	all := s.repo.List()
	out := make([]Skill, 0, len(all))
	for _, item := range all {
		if s.IsActive(item.Name) {
			out = append(out, item)
		}
	}
	return out
}

// ActiveSkillNames returns active skill names in deterministic order.
func (s *ActivationState) ActiveSkillNames() []string {
	active := s.ActiveSkills()
	out := make([]string, 0, len(active))
	for _, item := range active {
		out = append(out, item.Name)
	}
	return out
}

// ActiveReferences returns active references for one skill in deterministic path order.
func (s *ActivationState) ActiveReferences(skillName string) []Reference {
	if s == nil || s.repo == nil {
		return nil
	}
	refs := s.repo.ListReferences(skillName)
	if len(refs) == 0 {
		return nil
	}
	activeSet := s.activeReferences[strings.TrimSpace(skillName)]
	if len(activeSet) == 0 {
		return nil
	}
	out := make([]Reference, 0, len(refs))
	for _, ref := range refs {
		if activeSet[ref.Path] {
			out = append(out, ref)
		}
	}
	return out
}

// Materialize returns deterministic system-context text for active skills and references.
func (s *ActivationState) Materialize() string {
	if s == nil || s.repo == nil {
		return ""
	}
	active := s.ActiveSkills()
	if len(active) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Loaded skills:\n")
	for _, item := range active {
		b.WriteString("\n## ")
		b.WriteString(item.Name)
		if item.Description != "" {
			b.WriteString("\n")
			b.WriteString(item.Description)
		}
		if strings.TrimSpace(item.Body) != "" {
			b.WriteString("\n\n")
			b.WriteString(strings.TrimSpace(item.Body))
		}
		refs := s.ActiveReferences(item.Name)
		if len(refs) > 0 {
			for _, ref := range refs {
				b.WriteString("\n\n### ")
				b.WriteString(ref.Path)
				if strings.TrimSpace(ref.Body) != "" {
					b.WriteString("\n")
					b.WriteString(strings.TrimSpace(ref.Body))
				}
			}
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}
