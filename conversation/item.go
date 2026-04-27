package conversation

import (
	"fmt"

	"github.com/codewandler/llmadapter/unified"
)

type ItemKind string

const (
	ItemMessage         ItemKind = "message"
	ItemAssistantTurn   ItemKind = "assistant_turn"
	ItemToolCall        ItemKind = "tool_call"
	ItemToolResult      ItemKind = "tool_result"
	ItemReasoning       ItemKind = "reasoning"
	ItemContinuation    ItemKind = "continuation"
	ItemContextFragment ItemKind = "context_fragment"
	ItemCompaction      ItemKind = "compaction"
	ItemAnnotation      ItemKind = "annotation"
)

type Item struct {
	Kind         ItemKind
	NodeID       NodeID
	ParentNodeID NodeID
	BranchID     BranchID
	Message      unified.Message
	ToolCall     unified.ToolCall
	ToolResult   unified.ToolResult
	Reasoning    unified.ReasoningPart
	Continuation ProviderContinuation
	Assistant    *AssistantTurnEvent
	Payload      Payload
}

func ProjectItems(tree *Tree, branch BranchID) ([]Item, error) {
	if tree == nil {
		return nil, fmt.Errorf("conversation: tree is nil")
	}
	path, err := tree.Path(branch)
	if err != nil {
		return nil, err
	}
	items := make([]Item, 0, len(path))
	for _, node := range path {
		item := Item{
			NodeID:       node.ID,
			ParentNodeID: node.Parent,
			BranchID:     branch,
			Payload:      node.Payload,
		}
		switch payload := node.Payload.(type) {
		case MessageEvent:
			item.Kind = ItemMessage
			item.Message = payload.Message
		case *MessageEvent:
			if payload == nil {
				continue
			}
			item.Kind = ItemMessage
			item.Message = payload.Message
		case AssistantTurnEvent:
			item.Kind = ItemAssistantTurn
			assistant := payload
			item.Assistant = &assistant
			item.Message = payload.Message
		case *AssistantTurnEvent:
			if payload == nil {
				continue
			}
			item.Kind = ItemAssistantTurn
			item.Assistant = payload
			item.Message = payload.Message
		case CompactionEvent, *CompactionEvent:
			item.Kind = ItemCompaction
		case AnnotationEvent, *AnnotationEvent:
			item.Kind = ItemAnnotation
		default:
			continue
		}
		items = append(items, item)
	}
	return items, nil
}

func MessagesFromItems(items []Item) []unified.Message {
	normalized := NormalizeItems(items)
	messages := make([]unified.Message, 0, len(normalized))
	for _, item := range normalized {
		switch item.Kind {
		case ItemMessage, ItemAssistantTurn, ItemContextFragment:
			messages = append(messages, sanitizeMessageForRequest(item.Message))
		}
	}
	return messages
}

func ExpandItems(items []Item) []Item {
	out := make([]Item, 0, len(items))
	for _, item := range items {
		switch item.Kind {
		case ItemMessage, ItemAssistantTurn, ItemContextFragment:
			out = append(out, item)
			for _, reasoning := range reasoningParts(item.Message.Content) {
				derived := item
				derived.Kind = ItemReasoning
				derived.Message = unified.Message{}
				derived.Reasoning = reasoning
				derived.ToolCall = unified.ToolCall{}
				derived.ToolResult = unified.ToolResult{}
				derived.Continuation = ProviderContinuation{}
				out = append(out, derived)
			}
			for _, call := range item.Message.ToolCalls {
				derived := item
				derived.Kind = ItemToolCall
				derived.Message = unified.Message{}
				derived.ToolCall = call
				derived.ToolResult = unified.ToolResult{}
				derived.Reasoning = unified.ReasoningPart{}
				derived.Continuation = ProviderContinuation{}
				out = append(out, derived)
			}
			for _, result := range item.Message.ToolResults {
				derived := item
				derived.Kind = ItemToolResult
				derived.Message = unified.Message{}
				derived.ToolCall = unified.ToolCall{}
				derived.ToolResult = result
				derived.Reasoning = unified.ReasoningPart{}
				derived.Continuation = ProviderContinuation{}
				out = append(out, derived)
			}
			if item.Assistant != nil {
				for _, continuation := range item.Assistant.Continuations {
					derived := item
					derived.Kind = ItemContinuation
					derived.Message = unified.Message{}
					derived.ToolCall = unified.ToolCall{}
					derived.ToolResult = unified.ToolResult{}
					derived.Reasoning = unified.ReasoningPart{}
					derived.Continuation = continuation
					out = append(out, derived)
				}
			}
		default:
			out = append(out, item)
		}
	}
	return out
}

func NormalizeItems(items []Item) []Item {
	expanded := ExpandItems(items)
	messages := make([]Item, 0, len(expanded))
	pendingToolCalls := map[string]unified.ToolCall{}
	for _, item := range expanded {
		if item.Kind != ItemMessage && item.Kind != ItemAssistantTurn && item.Kind != ItemContextFragment {
			continue
		}
		msg := sanitizeMessageForRequest(item.Message)
		msg.Content = sanitizeContentParts(msg.Content)
		if msg.Role == unified.RoleAssistant {
			msg.ToolCalls = sanitizeToolCalls(msg.ToolCalls)
			for _, call := range msg.ToolCalls {
				if call.ID != "" {
					pendingToolCalls[call.ID] = call
				}
			}
		}
		if msg.Role == unified.RoleTool {
			msg.ToolResults = sanitizeToolResults(msg.ToolResults, pendingToolCalls)
			for _, result := range msg.ToolResults {
				delete(pendingToolCalls, result.ToolCallID)
			}
			if len(msg.ToolResults) == 0 && len(msg.Content) == 0 {
				continue
			}
		}
		item.Message = msg
		messages = append(messages, item)
	}
	if len(pendingToolCalls) == 0 {
		return messages
	}
	out := make([]Item, 0, len(messages)+len(pendingToolCalls))
	for _, item := range messages {
		out = append(out, item)
		if item.Kind != ItemAssistantTurn && item.Kind != ItemMessage {
			continue
		}
		if item.Message.Role != unified.RoleAssistant {
			continue
		}
		var missing []unified.ToolResult
		for _, call := range item.Message.ToolCalls {
			if call.ID == "" {
				continue
			}
			if _, ok := pendingToolCalls[call.ID]; !ok {
				continue
			}
			missing = append(missing, unified.ToolResult{
				ToolCallID: call.ID,
				Name:       call.Name,
				Content:    []unified.ContentPart{unified.TextPart{Text: "[Tool result missing]"}},
				IsError:    true,
			})
			delete(pendingToolCalls, call.ID)
		}
		if len(missing) > 0 {
			out = append(out, Item{
				Kind: ItemMessage,
				Message: unified.Message{
					Role:        unified.RoleTool,
					ToolResults: missing,
				},
			})
		}
	}
	return out
}

func sanitizeContentParts(parts []unified.ContentPart) []unified.ContentPart {
	out := make([]unified.ContentPart, 0, len(parts))
	for _, part := range parts {
		switch part := part.(type) {
		case unified.ImagePart:
			if part.Source.URL == "" && part.Source.Base64 == "" {
				continue
			}
		case *unified.ImagePart:
			if part == nil || (part.Source.URL == "" && part.Source.Base64 == "") {
				continue
			}
		case unified.FilePart:
			if part.Source.URL == "" && part.Source.Base64 == "" {
				continue
			}
		case *unified.FilePart:
			if part == nil || (part.Source.URL == "" && part.Source.Base64 == "") {
				continue
			}
		}
		out = append(out, part)
	}
	return out
}

func sanitizeToolCalls(calls []unified.ToolCall) []unified.ToolCall {
	out := make([]unified.ToolCall, 0, len(calls))
	for _, call := range calls {
		if call.ID == "" || call.Name == "" {
			continue
		}
		out = append(out, call)
	}
	return out
}

func sanitizeToolResults(results []unified.ToolResult, pending map[string]unified.ToolCall) []unified.ToolResult {
	out := make([]unified.ToolResult, 0, len(results))
	for _, result := range results {
		if result.ToolCallID == "" {
			continue
		}
		if _, ok := pending[result.ToolCallID]; !ok {
			continue
		}
		result.Content = sanitizeContentParts(result.Content)
		out = append(out, result)
	}
	return out
}

func reasoningParts(parts []unified.ContentPart) []unified.ReasoningPart {
	var out []unified.ReasoningPart
	for _, part := range parts {
		switch part := part.(type) {
		case unified.ReasoningPart:
			out = append(out, part)
		case *unified.ReasoningPart:
			if part != nil {
				out = append(out, *part)
			}
		}
	}
	return out
}
