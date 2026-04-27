package capability

import (
	"encoding/json"
	"fmt"
	"reflect"
)

// StateEventDefinition describes one registered inner state event for a
// capability type. The durable outer event remains
// capability.state_event_dispatched; definitions validate the inner event name
// and JSON body before replay applies it to materialized state.
type StateEventDefinition struct {
	CapabilityName string
	EventName      string
	BodyType       reflect.Type
}

// DefineStateEvent creates a low-boilerplate definition for a typed capability
// state event body.
func DefineStateEvent[T any](capabilityName, eventName string) StateEventDefinition {
	return StateEventDefinition{
		CapabilityName: capabilityName,
		EventName:      eventName,
		BodyType:       reflect.TypeOf((*T)(nil)).Elem(),
	}
}

// StateEventDefinitions is optionally implemented by capability factories that
// want replay-time validation of dispatched inner events.
type StateEventDefinitions interface {
	StateEventDefinitions() []StateEventDefinition
}

type stateEventRegistry struct {
	definitions map[string]map[string]StateEventDefinition
}

func newStateEventRegistry() *stateEventRegistry {
	return &stateEventRegistry{definitions: map[string]map[string]StateEventDefinition{}}
}

func (r *stateEventRegistry) Register(definitions ...StateEventDefinition) error {
	if r.definitions == nil {
		r.definitions = map[string]map[string]StateEventDefinition{}
	}
	for _, definition := range definitions {
		if definition.CapabilityName == "" {
			return fmt.Errorf("capability: state event capability name is required")
		}
		if definition.EventName == "" {
			return fmt.Errorf("capability: state event name is required")
		}
		events := r.definitions[definition.CapabilityName]
		if events == nil {
			events = map[string]StateEventDefinition{}
			r.definitions[definition.CapabilityName] = events
		}
		if _, exists := events[definition.EventName]; exists {
			return fmt.Errorf("capability: state event %s/%s already registered", definition.CapabilityName, definition.EventName)
		}
		events[definition.EventName] = definition
	}
	return nil
}

func (r *stateEventRegistry) Validate(capabilityName string, event StateEvent) error {
	if r == nil || len(r.definitions) == 0 {
		return nil
	}
	events := r.definitions[capabilityName]
	if len(events) == 0 {
		return nil
	}
	definition, ok := events[event.Name]
	if !ok {
		return fmt.Errorf("capability: state event %s/%s is not registered", capabilityName, event.Name)
	}
	if definition.BodyType == nil {
		return nil
	}
	target := reflect.New(definition.BodyType)
	if err := json.Unmarshal(event.Body, target.Interface()); err != nil {
		return fmt.Errorf("capability: validate state event %s/%s: %w", capabilityName, event.Name, err)
	}
	return nil
}
