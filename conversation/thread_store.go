package conversation

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/codewandler/agentsdk/thread"
)

const EventConversationStored thread.EventKind = "conversation.event_stored"

type ThreadEventStore struct {
	store thread.Store
	live  thread.Live
}

type threadConversationRecord struct {
	Kind           StructuralEventKind `json:"kind"`
	ConversationID ConversationID      `json:"conversation_id,omitempty"`
	SessionID      SessionID           `json:"session_id,omitempty"`
	BranchID       BranchID            `json:"branch_id,omitempty"`
	NodeID         NodeID              `json:"node_id,omitempty"`
	ParentNodeID   NodeID              `json:"parent_node_id,omitempty"`
	FromBranchID   BranchID            `json:"from_branch_id,omitempty"`
	PayloadKind    PayloadKind         `json:"payload_kind,omitempty"`
	Payload        json.RawMessage     `json:"payload,omitempty"`
	At             time.Time           `json:"at"`
}

func NewThreadEventStore(store thread.Store, live thread.Live) *ThreadEventStore {
	return &ThreadEventStore{store: store, live: live}
}

func (s *ThreadEventStore) AppendEvents(ctx context.Context, events ...Event) error {
	if s == nil || s.live == nil {
		return fmt.Errorf("conversation: live thread is required")
	}
	threadEvents := make([]thread.Event, 0, len(events))
	for _, event := range events {
		payload, err := encodeThreadConversationRecord(event)
		if err != nil {
			return err
		}
		threadEvents = append(threadEvents, thread.Event{
			Kind:         EventConversationStored,
			BranchID:     thread.BranchID(event.BranchID),
			NodeID:       thread.NodeID(event.NodeID),
			ParentNodeID: thread.NodeID(event.ParentNodeID),
			Payload:      payload,
			At:           event.At,
			Source:       thread.EventSource{Type: "conversation", SessionID: string(event.SessionID)},
		})
	}
	return s.live.Append(ctx, threadEvents...)
}

func (s *ThreadEventStore) LoadEvents(ctx context.Context, conversationID ConversationID) ([]Event, error) {
	if s == nil || s.store == nil || s.live == nil {
		return nil, fmt.Errorf("conversation: thread store and live thread are required")
	}
	stored, err := s.store.Read(ctx, thread.ReadParams{ID: s.live.ID()})
	if err != nil {
		return nil, err
	}
	branchEvents, err := stored.EventsForBranch(s.live.BranchID())
	if err != nil {
		return nil, err
	}
	var out []Event
	for _, threadEvent := range branchEvents {
		if threadEvent.Kind != EventConversationStored {
			continue
		}
		event, err := decodeThreadConversationRecord(threadEvent.Payload)
		if err != nil {
			return nil, err
		}
		if conversationID == "" || event.ConversationID == conversationID {
			out = append(out, event)
		}
	}
	return out, nil
}

func encodeThreadConversationRecord(event Event) (json.RawMessage, error) {
	record := threadConversationRecord{
		Kind:           event.Kind,
		ConversationID: event.ConversationID,
		SessionID:      event.SessionID,
		BranchID:       event.BranchID,
		NodeID:         event.NodeID,
		ParentNodeID:   event.ParentNodeID,
		FromBranchID:   event.FromBranchID,
		At:             event.At,
	}
	if event.Payload != nil {
		kind, payload, err := MarshalPayload(event.Payload)
		if err != nil {
			return nil, err
		}
		record.PayloadKind = kind
		record.Payload = payload
	}
	return json.Marshal(record)
}

func decodeThreadConversationRecord(raw json.RawMessage) (Event, error) {
	var record threadConversationRecord
	if err := json.Unmarshal(raw, &record); err != nil {
		return Event{}, err
	}
	var payload Payload
	if record.PayloadKind != "" {
		decoded, err := UnmarshalPayload(record.PayloadKind, record.Payload)
		if err != nil {
			return Event{}, err
		}
		payload = decoded
	}
	return Event{
		Kind:           record.Kind,
		ConversationID: record.ConversationID,
		SessionID:      record.SessionID,
		BranchID:       record.BranchID,
		NodeID:         record.NodeID,
		ParentNodeID:   record.ParentNodeID,
		FromBranchID:   record.FromBranchID,
		Payload:        payload,
		At:             record.At,
	}, nil
}
