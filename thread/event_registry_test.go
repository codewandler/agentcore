package thread

import (
	"context"
	"encoding/json"
	"testing"
)

func TestEventRegistryValidatesRegisteredPayloads(t *testing.T) {
	registry, err := NewEventRegistry(DefineEvent[BranchCreatedPayload](EventBranchCreated))
	if err != nil {
		t.Fatal(err)
	}
	valid, err := json.Marshal(BranchCreatedPayload{FromBranchID: MainBranch, ToBranchID: "alt", ForkSeq: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Validate(Event{Kind: EventBranchCreated, Payload: valid}); err != nil {
		t.Fatal(err)
	}
	if err := registry.Validate(Event{Kind: EventBranchCreated, Payload: json.RawMessage(`{`)}); err == nil {
		t.Fatal("expected invalid payload to fail validation")
	}
	if err := registry.Validate(Event{Kind: "plugin.unregistered", Payload: json.RawMessage(`{`)}); err != nil {
		t.Fatalf("unregistered events should remain open: %v", err)
	}
}

func TestMemoryStoreValidatesRegisteredEventsOnAppendAndImport(t *testing.T) {
	registry, err := NewEventRegistry(CoreEventDefinitions()...)
	if err != nil {
		t.Fatal(err)
	}
	store := NewMemoryStore(WithEventRegistry(registry))
	live, err := store.Create(context.Background(), CreateParams{ID: "thread_registry"})
	if err != nil {
		t.Fatal(err)
	}
	if err := live.Append(context.Background(), Event{Kind: EventBranchCreated, Payload: json.RawMessage(`{`)}); err == nil {
		t.Fatal("expected append validation error")
	}
	if err := store.Import(context.Background(), Event{
		ThreadID: "thread_import",
		Kind:     EventBranchCreated,
		Payload:  json.RawMessage(`{`),
	}); err == nil {
		t.Fatal("expected import validation error")
	}
}
