package thread

import (
	"encoding/json"
	"fmt"
	"reflect"
)

type EventDefinition struct {
	Kind              EventKind
	PayloadType       reflect.Type
	AllowEmptyPayload bool
}

func DefineEvent[T any](kind EventKind) EventDefinition {
	return EventDefinition{
		Kind:        kind,
		PayloadType: reflect.TypeOf((*T)(nil)).Elem(),
	}
}

func DefineEmptyEvent(kind EventKind) EventDefinition {
	return EventDefinition{Kind: kind, AllowEmptyPayload: true}
}

type EventRegistry interface {
	Register(...EventDefinition) error
	Validate(Event) error
}

type MemoryEventRegistry struct {
	definitions map[EventKind]EventDefinition
}

func NewEventRegistry(definitions ...EventDefinition) (*MemoryEventRegistry, error) {
	registry := &MemoryEventRegistry{definitions: map[EventKind]EventDefinition{}}
	if err := registry.Register(definitions...); err != nil {
		return nil, err
	}
	return registry, nil
}

func (r *MemoryEventRegistry) Register(definitions ...EventDefinition) error {
	if r.definitions == nil {
		r.definitions = map[EventKind]EventDefinition{}
	}
	for _, definition := range definitions {
		if definition.Kind == "" {
			return fmt.Errorf("thread: event definition kind is required")
		}
		if _, exists := r.definitions[definition.Kind]; exists {
			return fmt.Errorf("thread: event definition %q already registered", definition.Kind)
		}
		r.definitions[definition.Kind] = definition
	}
	return nil
}

func (r *MemoryEventRegistry) Validate(event Event) error {
	if event.Kind == "" {
		return fmt.Errorf("thread: event kind is required")
	}
	if r == nil || len(r.definitions) == 0 {
		return nil
	}
	definition, ok := r.definitions[event.Kind]
	if !ok {
		return nil
	}
	if len(event.Payload) == 0 {
		if definition.AllowEmptyPayload || definition.PayloadType == nil {
			return nil
		}
		return fmt.Errorf("thread: event %q payload is required", event.Kind)
	}
	if definition.PayloadType == nil {
		return nil
	}
	target := reflect.New(definition.PayloadType)
	if err := json.Unmarshal(event.Payload, target.Interface()); err != nil {
		return fmt.Errorf("thread: validate event %q: %w", event.Kind, err)
	}
	return nil
}

func CoreEventDefinitions() []EventDefinition {
	return []EventDefinition{
		DefineEvent[ThreadCreatedPayload](EventThreadCreated),
		DefineEmptyEvent(EventMetadataUpdated),
		DefineEmptyEvent(EventThreadArchived),
		DefineEmptyEvent(EventThreadUnarchived),
		DefineEvent[BranchCreatedPayload](EventBranchCreated),
		DefineEmptyEvent(EventBranchHeadMoved),
	}
}

type ThreadCreatedPayload struct {
	Metadata map[string]string `json:"metadata,omitempty"`
}

type BranchCreatedPayload struct {
	FromBranchID BranchID `json:"from_branch_id"`
	ToBranchID   BranchID `json:"to_branch_id"`
	ForkSeq      int64    `json:"fork_seq"`
}
