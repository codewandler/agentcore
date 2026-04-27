package conversation

import (
	"fmt"

	"github.com/codewandler/llmadapter/unified"
)

type ItemKind string

const (
	ItemMessage         ItemKind = "message"
	ItemAssistantTurn   ItemKind = "assistant_turn"
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
	messages := make([]unified.Message, 0, len(items))
	for _, item := range items {
		switch item.Kind {
		case ItemMessage, ItemAssistantTurn, ItemContextFragment:
			messages = append(messages, sanitizeMessageForRequest(item.Message))
		}
	}
	return messages
}
