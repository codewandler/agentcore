package conversation

import (
	"strings"

	"github.com/codewandler/llmadapter/unified"
)

func ProjectMessages(tree *Tree, branch BranchID) ([]unified.Message, error) {
	items, err := ProjectItems(tree, branch)
	if err != nil {
		return nil, err
	}
	return MessagesFromItems(items), nil
}

func sanitizeMessageForRequest(msg unified.Message) unified.Message {
	if msg.Role == unified.RoleAssistant && strings.HasPrefix(msg.ID, "resp_") {
		msg.ID = ""
	}
	if msg.Role == unified.RoleAssistant {
		msg.Content = sanitizeAssistantContentForRequest(msg.Content)
	}
	return msg
}

func sanitizeAssistantContentForRequest(parts []unified.ContentPart) []unified.ContentPart {
	out := make([]unified.ContentPart, 0, len(parts))
	for _, part := range parts {
		switch part := part.(type) {
		case unified.ReasoningPart:
			if part.Signature == "" {
				continue
			}
		case *unified.ReasoningPart:
			if part == nil || part.Signature == "" {
				continue
			}
		}
		out = append(out, part)
	}
	return out
}
