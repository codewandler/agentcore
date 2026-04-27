package conversation

import (
	"strings"

	"github.com/codewandler/llmadapter/unified"
)

func AddCodexSessionHints(req *unified.Request, identity ProviderIdentity, sessionID string, tree *Tree, branch BranchID) error {
	head, _ := tree.Head(branch)
	hints := unified.CodexExtensions{
		InteractionMode: unified.InteractionSession,
		SessionID:       sessionID,
		BranchID:        string(branch),
		BranchHeadID:    string(head),
	}
	if continuation, ok, err := ContinuationAtBranchHead(tree, branch, identity); err == nil && ok {
		hints.ParentResponseID = continuation.ResponseID
	} else if err != nil {
		return err
	}
	return unified.SetCodexExtensions(&req.Extensions, hints)
}

func IsCodexResponsesIdentity(identity ProviderIdentity) bool {
	provider := strings.ToLower(strings.TrimSpace(identity.ProviderName))
	apiKind := strings.ToLower(strings.TrimSpace(identity.APIKind))
	apiFamily := strings.ToLower(strings.TrimSpace(identity.APIFamily))
	return strings.Contains(provider, "codex") ||
		strings.Contains(apiKind, "codex") ||
		strings.Contains(apiFamily, "codex")
}
