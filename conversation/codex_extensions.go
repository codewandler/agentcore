package conversation

import (
	"strings"

	"github.com/codewandler/llmadapter/unified"
)

func (s *Session) addCodexSessionHints(req *unified.Request, identity ProviderIdentity) error {
	sessionID := string(s.conversationID)
	if sessionID == "" {
		sessionID = string(s.sessionID)
	}
	head, _ := s.tree.Head(s.branch)

	hints := unified.CodexExtensions{
		InteractionMode: unified.InteractionSession,
		SessionID:       sessionID,
		BranchID:        string(s.branch),
		BranchHeadID:    string(head),
	}
	if continuation, ok, err := ContinuationAtBranchHead(s.tree, s.branch, identity); err == nil && ok {
		hints.ParentResponseID = continuation.ResponseID
	} else if err != nil {
		return err
	}
	return unified.SetCodexExtensions(&req.Extensions, hints)
}

func isCodexResponsesIdentity(identity ProviderIdentity) bool {
	provider := strings.ToLower(strings.TrimSpace(identity.ProviderName))
	apiKind := strings.ToLower(strings.TrimSpace(identity.APIKind))
	apiFamily := strings.ToLower(strings.TrimSpace(identity.APIFamily))
	return strings.Contains(provider, "codex") ||
		strings.Contains(apiKind, "codex") ||
		strings.Contains(apiFamily, "codex")
}
