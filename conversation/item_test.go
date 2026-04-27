package conversation

import (
	"testing"

	"github.com/codewandler/llmadapter/unified"
)

func TestProjectItemsNormalizesMessageAndAssistantTurns(t *testing.T) {
	session := New()
	if _, err := session.AppendMessage(unified.Message{Role: unified.RoleUser, Content: []unified.ContentPart{unified.TextPart{Text: "hi"}}}); err != nil {
		t.Fatal(err)
	}
	fragment := NewTurnFragment()
	fragment.SetAssistantMessage(unified.Message{Role: unified.RoleAssistant, Content: []unified.ContentPart{unified.TextPart{Text: "hello"}}})
	fragment.Complete(unified.FinishReasonStop)
	if _, err := session.CommitFragment(fragment); err != nil {
		t.Fatal(err)
	}

	items, err := ProjectItems(session.Tree(), session.Branch())
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(items), 2; got != want {
		t.Fatalf("items = %d, want %d", got, want)
	}
	if items[0].Kind != ItemMessage || items[1].Kind != ItemAssistantTurn {
		t.Fatalf("item kinds = %q %q", items[0].Kind, items[1].Kind)
	}
	messages := MessagesFromItems(items)
	if got, want := len(messages), 2; got != want {
		t.Fatalf("messages = %d, want %d", got, want)
	}
}
