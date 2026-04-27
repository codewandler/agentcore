package jsonlstore

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/codewandler/agentsdk/thread"
)

func TestStorePersistsCreateAppendResumeFork(t *testing.T) {
	ctx := context.Background()
	store := Open(t.TempDir())
	live, err := store.Create(ctx, thread.CreateParams{ID: "thread_jsonl"})
	if err != nil {
		t.Fatal(err)
	}
	if err := live.Append(ctx, thread.Event{Kind: "capability.attached", Payload: json.RawMessage(`{"instance_id":"planner_1"}`)}); err != nil {
		t.Fatal(err)
	}
	alt, err := store.Fork(ctx, thread.ForkParams{ID: live.ID(), FromBranchID: thread.MainBranch, ToBranchID: "alt"})
	if err != nil {
		t.Fatal(err)
	}
	if err := alt.Append(ctx, thread.Event{Kind: "capability.state_event_dispatched", Payload: json.RawMessage(`{"branch":"alt"}`)}); err != nil {
		t.Fatal(err)
	}

	reopened := Open(store.dir)
	resumed, err := reopened.Resume(ctx, thread.ResumeParams{ID: live.ID(), BranchID: "alt"})
	if err != nil {
		t.Fatal(err)
	}
	if resumed.BranchID() != "alt" {
		t.Fatalf("branch = %q, want alt", resumed.BranchID())
	}
	stored, err := reopened.Read(ctx, thread.ReadParams{ID: live.ID()})
	if err != nil {
		t.Fatal(err)
	}
	events, err := stored.EventsForBranch("alt")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(events), 4; got != want {
		t.Fatalf("alt events = %d, want %d", got, want)
	}
}
