package workflow

import (
	"context"
	"sync"
)

// RunStore records workflow run events and projects their current state.
// Implementations are responsible for isolating runs by RunID and for not
// exposing mutable internal event slices to callers.
type RunStore interface {
	Append(ctx context.Context, runID RunID, events ...any) error
	Events(ctx context.Context, runID RunID) ([]any, bool, error)
	State(ctx context.Context, runID RunID) (RunState, bool, error)
}

// MemoryRunStore is an in-memory RunStore implementation. It retains all events
// appended for a run; State relies on Projector to ignore non-workflow events
// and report unsupported workflow event shapes.
type MemoryRunStore struct {
	mu     sync.RWMutex
	events map[RunID][]any
}

// NewMemoryRunStore returns an empty in-memory workflow run store.
func NewMemoryRunStore() *MemoryRunStore {
	return &MemoryRunStore{events: map[RunID][]any{}}
}

// Append records events for runID. The event slice supplied by the caller is
// copied before storage so later caller-side slice mutations do not affect the
// store.
func (s *MemoryRunStore) Append(ctx context.Context, runID RunID, events ...any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(events) == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.events == nil {
		s.events = map[RunID][]any{}
	}
	s.events[runID] = append(s.events[runID], copyEvents(events)...)
	return nil
}

// Events returns a copy of the events recorded for runID.
func (s *MemoryRunStore) Events(ctx context.Context, runID RunID) ([]any, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	events, ok := s.events[runID]
	if !ok {
		return nil, false, nil
	}
	return copyEvents(events), true, nil
}

// State projects the current state for runID from its recorded events.
func (s *MemoryRunStore) State(ctx context.Context, runID RunID) (RunState, bool, error) {
	events, ok, err := s.Events(ctx, runID)
	if err != nil || !ok {
		return RunState{}, ok, err
	}
	return Projector{}.ProjectRun(events, runID)
}

func copyEvents(events []any) []any {
	if len(events) == 0 {
		return nil
	}
	copied := make([]any, len(events))
	copy(copied, events)
	return copied
}
