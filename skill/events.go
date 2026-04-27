package skill

import "github.com/codewandler/agentsdk/thread"

const (
	EventSkillActivated          thread.EventKind = "harness.skill_activated"
	EventSkillReferenceActivated thread.EventKind = "harness.skill_reference_activated"
)

type SkillActivatedEvent struct {
	Skill string `json:"skill"`
}

type SkillReferenceActivatedEvent struct {
	Skill string `json:"skill"`
	Path  string `json:"path"`
}
