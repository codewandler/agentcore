package conversation

import (
	"context"
	"testing"

	"github.com/codewandler/agentsdk/thread"
	"github.com/codewandler/llmadapter/unified"
)

func TestThreadEventStorePersistsSessionEvents(t *testing.T) {
	ctx := context.Background()
	store := thread.NewMemoryStore()
	live, err := store.Create(ctx, thread.CreateParams{ID: "thread_conversation"})
	if err != nil {
		t.Fatal(err)
	}
	events := NewThreadEventStore(store, live)
	session := New(WithConversationID("conversation_1"), WithStore(events))
	if _, err := session.AppendMessage(unified.Message{Role: unified.RoleUser, Content: []unified.ContentPart{unified.TextPart{Text: "hi"}}}); err != nil {
		t.Fatal(err)
	}

	resumed, err := Resume(ctx, events, "conversation_1")
	if err != nil {
		t.Fatal(err)
	}
	messages, err := resumed.Messages()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(messages), 1; got != want {
		t.Fatalf("messages = %d, want %d", got, want)
	}
	if messages[0].Content[0].(unified.TextPart).Text != "hi" {
		t.Fatalf("message = %#v", messages[0])
	}
}
