package conversation

import (
	"testing"

	"github.com/codewandler/llmadapter/unified"
)

func TestProjectItemsNormalizesMessageAndAssistantTurns(t *testing.T) {
	tree := NewTree()
	if _, err := tree.Append(MainBranch, MessageEvent{Message: unified.Message{Role: unified.RoleUser, Content: []unified.ContentPart{unified.TextPart{Text: "hi"}}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := tree.Append(MainBranch, AssistantTurnEvent{Message: unified.Message{Role: unified.RoleAssistant, Content: []unified.ContentPart{unified.TextPart{Text: "hello"}}}, FinishReason: unified.FinishReasonStop}); err != nil {
		t.Fatal(err)
	}

	items, err := ProjectItems(tree, MainBranch)
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

func TestNormalizeItemsInsertsMissingToolResults(t *testing.T) {
	items := []Item{{
		Kind: ItemAssistantTurn,
		Message: unified.Message{
			Role: unified.RoleAssistant,
			ToolCalls: []unified.ToolCall{{
				ID:   "call_1",
				Name: "plan",
			}},
		},
	}}

	messages := MessagesFromItems(items)
	if got, want := len(messages), 2; got != want {
		t.Fatalf("messages = %d, want %d", got, want)
	}
	if messages[1].Role != unified.RoleTool || len(messages[1].ToolResults) != 1 {
		t.Fatalf("missing synthetic tool result: %#v", messages[1])
	}
	result := messages[1].ToolResults[0]
	if result.ToolCallID != "call_1" || !result.IsError {
		t.Fatalf("synthetic result = %#v", result)
	}
}

func TestNormalizeItemsDropsOrphanToolResults(t *testing.T) {
	items := []Item{{
		Kind: ItemMessage,
		Message: unified.Message{
			Role: unified.RoleTool,
			ToolResults: []unified.ToolResult{{
				ToolCallID: "orphan",
				Name:       "plan",
				Content:    []unified.ContentPart{unified.TextPart{Text: "ignored"}},
			}},
		},
	}}

	messages := MessagesFromItems(items)
	if len(messages) != 0 {
		t.Fatalf("messages = %#v, want orphan tool result dropped", messages)
	}
}

func TestNormalizeItemsDropsDuplicateToolCallsAndResults(t *testing.T) {
	items := []Item{
		{
			Kind: ItemAssistantTurn,
			Message: unified.Message{
				Role: unified.RoleAssistant,
				ToolCalls: []unified.ToolCall{
					{ID: "call_1", Name: "plan"},
					{ID: "call_1", Name: "plan_duplicate"},
				},
			},
		},
		{
			Kind: ItemMessage,
			Message: unified.Message{
				Role: unified.RoleTool,
				ToolResults: []unified.ToolResult{
					{ToolCallID: "call_1", Name: "plan", Content: []unified.ContentPart{unified.TextPart{Text: "ok"}}},
					{ToolCallID: "call_1", Name: "plan", Content: []unified.ContentPart{unified.TextPart{Text: "duplicate"}}},
				},
			},
		},
	}

	messages := MessagesFromItems(items)
	if got, want := len(messages), 2; got != want {
		t.Fatalf("messages = %d, want %d: %#v", got, want, messages)
	}
	if got, want := len(messages[0].ToolCalls), 1; got != want {
		t.Fatalf("tool calls = %d, want %d", got, want)
	}
	if got, want := messages[0].ToolCalls[0].Name, "plan"; got != want {
		t.Fatalf("tool call name = %q, want %q", got, want)
	}
	if got, want := len(messages[1].ToolResults), 1; got != want {
		t.Fatalf("tool results = %d, want %d", got, want)
	}
	text, ok := messages[1].ToolResults[0].Content[0].(unified.TextPart)
	if !ok || text.Text != "ok" {
		t.Fatalf("tool result content = %#v", messages[1].ToolResults[0].Content)
	}
}

func TestNormalizeItemsDropsRepeatedToolCallAfterCompletion(t *testing.T) {
	items := []Item{
		{
			Kind: ItemAssistantTurn,
			Message: unified.Message{
				Role:      unified.RoleAssistant,
				ToolCalls: []unified.ToolCall{{ID: "call_1", Name: "plan"}},
			},
		},
		{
			Kind: ItemMessage,
			Message: unified.Message{
				Role: unified.RoleTool,
				ToolResults: []unified.ToolResult{{
					ToolCallID: "call_1",
					Name:       "plan",
					Content:    []unified.ContentPart{unified.TextPart{Text: "ok"}},
				}},
			},
		},
		{
			Kind: ItemAssistantTurn,
			Message: unified.Message{
				Role:      unified.RoleAssistant,
				ToolCalls: []unified.ToolCall{{ID: "call_1", Name: "plan_again"}},
			},
		},
	}

	messages := MessagesFromItems(items)
	if got, want := len(messages), 2; got != want {
		t.Fatalf("messages = %d, want %d: %#v", got, want, messages)
	}
	if got, want := len(messages[0].ToolCalls), 1; got != want {
		t.Fatalf("tool calls = %d, want %d", got, want)
	}
	if got, want := len(messages[1].ToolResults), 1; got != want {
		t.Fatalf("tool results = %d, want %d", got, want)
	}
}

func TestNormalizeItemsDropsAssistantMessageWithOnlyDuplicateToolCalls(t *testing.T) {
	items := []Item{
		{
			Kind: ItemAssistantTurn,
			Message: unified.Message{
				Role: unified.RoleAssistant,
				ToolCalls: []unified.ToolCall{
					{ID: "call_1", Name: "plan"},
					{ID: "call_1", Name: "plan_duplicate"},
				},
			},
		},
		{
			Kind: ItemMessage,
			Message: unified.Message{
				Role: unified.RoleTool,
				ToolResults: []unified.ToolResult{{
					ToolCallID: "call_1",
					Name:       "plan",
					Content:    []unified.ContentPart{unified.TextPart{Text: "ok"}},
				}},
			},
		},
		{
			Kind: ItemAssistantTurn,
			Message: unified.Message{
				Role: unified.RoleAssistant,
				ToolCalls: []unified.ToolCall{
					{ID: "call_1", Name: "plan_third"},
				},
			},
		},
	}

	messages := MessagesFromItems(items)
	if got, want := len(messages), 2; got != want {
		t.Fatalf("messages = %d, want %d: %#v", got, want, messages)
	}
}

func TestNormalizeItemsStripsUnsupportedMedia(t *testing.T) {
	items := []Item{{
		Kind: ItemMessage,
		Message: unified.Message{
			Role: unified.RoleUser,
			Content: []unified.ContentPart{
				unified.TextPart{Text: "keep"},
				unified.ImagePart{Alt: "empty"},
				unified.FilePart{Filename: "empty.txt"},
			},
		},
	}}

	messages := MessagesFromItems(items)
	if got, want := len(messages[0].Content), 1; got != want {
		t.Fatalf("content parts = %d, want %d: %#v", got, want, messages[0].Content)
	}
}

func TestExpandItemsDerivesToolReasoningAndContinuationItems(t *testing.T) {
	assistant := AssistantTurnEvent{
		Message: unified.Message{
			Role: unified.RoleAssistant,
			Content: []unified.ContentPart{
				unified.ReasoningPart{Text: "think", Signature: "sig"},
				unified.TextPart{Text: "done"},
			},
			ToolCalls: []unified.ToolCall{{ID: "call_1", Name: "plan"}},
		},
		Continuations: []ProviderContinuation{{ResponseID: "resp_1"}},
	}
	items := ExpandItems([]Item{{Kind: ItemAssistantTurn, Message: assistant.Message, Assistant: &assistant}})
	var sawReasoning, sawToolCall, sawContinuation bool
	for _, item := range items {
		switch item.Kind {
		case ItemReasoning:
			sawReasoning = item.Reasoning.Text == "think"
		case ItemToolCall:
			sawToolCall = item.ToolCall.ID == "call_1"
		case ItemContinuation:
			sawContinuation = item.Continuation.ResponseID == "resp_1"
		}
	}
	if !sawReasoning || !sawToolCall || !sawContinuation {
		t.Fatalf("derived items missing: reasoning=%v toolCall=%v continuation=%v items=%#v", sawReasoning, sawToolCall, sawContinuation, items)
	}
}

func TestProjectItemsPlacesCompactionAtReplacementWindow(t *testing.T) {
	tree := NewTree()
	oldOne, err := tree.Append(MainBranch, MessageEvent{Message: textMessage(unified.RoleUser, "old one")})
	if err != nil {
		t.Fatal(err)
	}
	oldTwo, err := tree.Append(MainBranch, MessageEvent{Message: textMessage(unified.RoleAssistant, "old two")})
	if err != nil {
		t.Fatal(err)
	}
	keep, err := tree.Append(MainBranch, MessageEvent{Message: textMessage(unified.RoleUser, "keep")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tree.Append(MainBranch, CompactionEvent{Summary: "summary", Replaces: []NodeID{oldOne, oldTwo}}); err != nil {
		t.Fatal(err)
	}

	messages, err := ProjectMessages(tree, MainBranch)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(messages), 2; got != want {
		t.Fatalf("messages = %d, want %d: %#v", got, want, messages)
	}
	requireTextPart(t, messages[0], "summary")
	requireTextPart(t, messages[1], "keep")
	for _, id := range []NodeID{oldOne, oldTwo, keep} {
		if _, ok := tree.Node(id); !ok {
			t.Fatalf("original node %q missing from tree", id)
		}
	}
}

func TestProjectItemsCompactionIsBranchLocal(t *testing.T) {
	tree := NewTree()
	old, err := tree.Append(MainBranch, MessageEvent{Message: textMessage(unified.RoleUser, "old")})
	if err != nil {
		t.Fatal(err)
	}
	if err := tree.Fork(MainBranch, "alt"); err != nil {
		t.Fatal(err)
	}
	if _, err := tree.Append(MainBranch, CompactionEvent{Summary: "summary", Replaces: []NodeID{old}}); err != nil {
		t.Fatal(err)
	}

	mainMessages, err := ProjectMessages(tree, MainBranch)
	if err != nil {
		t.Fatal(err)
	}
	requireTextPart(t, mainMessages[0], "summary")

	altMessages, err := ProjectMessages(tree, "alt")
	if err != nil {
		t.Fatal(err)
	}
	requireTextPart(t, altMessages[0], "old")
}

func textMessage(role unified.Role, text string) unified.Message {
	return unified.Message{Role: role, Content: []unified.ContentPart{unified.TextPart{Text: text}}}
}

func requireTextPart(t *testing.T, message unified.Message, want string) {
	t.Helper()
	if len(message.Content) != 1 {
		t.Fatalf("content = %#v, want one text part", message.Content)
	}
	text, ok := message.Content[0].(unified.TextPart)
	if !ok || text.Text != want {
		t.Fatalf("text = %#v, want %q", message.Content[0], want)
	}
}

func TestProjectItemsWithFloorExcludesReplacedNodes(t *testing.T) {
	tree := NewTree()
	old1, err := tree.Append(MainBranch, MessageEvent{Message: unified.Message{Role: unified.RoleUser, Content: []unified.ContentPart{unified.TextPart{Text: "old1"}}}})
	if err != nil {
		t.Fatal(err)
	}
	old2, err := tree.Append(MainBranch, MessageEvent{Message: unified.Message{Role: unified.RoleUser, Content: []unified.ContentPart{unified.TextPart{Text: "old2"}}}})
	if err != nil {
		t.Fatal(err)
	}
	keep, err := tree.Append(MainBranch, MessageEvent{Message: unified.Message{Role: unified.RoleUser, Content: []unified.ContentPart{unified.TextPart{Text: "keep"}}}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = tree.Append(MainBranch, CompactionEvent{Summary: "summary of old", Replaces: []NodeID{old1, old2}})
	if err != nil {
		t.Fatal(err)
	}

	// Without floor: ProjectItems should produce [summary, keep].
	items, err := ProjectItems(tree, MainBranch)
	if err != nil {
		t.Fatal(err)
	}
	messages := MessagesFromItems(items)
	if len(messages) != 2 {
		t.Fatalf("without floor: expected 2 messages, got %d", len(messages))
	}

	// Set floor to keep (earliest non-replaced node).
	tree.SetFloor(MainBranch, keep)

	// With floor: replaced nodes are excluded from Path, but ProjectItems
	// should still produce [summary, keep] with summary at the beginning.
	items, err = ProjectItems(tree, MainBranch)
	if err != nil {
		t.Fatal(err)
	}
	messages = MessagesFromItems(items)
	if len(messages) != 2 {
		t.Fatalf("with floor: expected 2 messages, got %d", len(messages))
	}
	if got := textFromMessage(messages[0]); got != "summary of old" {
		t.Fatalf("with floor: expected first message 'summary of old', got %q", got)
	}
	if got := textFromMessage(messages[1]); got != "keep" {
		t.Fatalf("with floor: expected second message 'keep', got %q", got)
	}
}

func textFromMessage(msg unified.Message) string {
	for _, part := range msg.Content {
		if tp, ok := part.(unified.TextPart); ok {
			return tp.Text
		}
	}
	return ""
}
