package conversation

import (
	"fmt"
	"strings"

	"github.com/codewandler/llmadapter/unified"
)

type ProviderIdentity struct {
	ProviderName string `json:"provider_name,omitempty"`
	APIKind      string `json:"api_kind,omitempty"`
	APIFamily    string `json:"api_family,omitempty"`
	NativeModel  string `json:"native_model,omitempty"`
}

type ProviderContinuation struct {
	ProviderName         string                   `json:"provider_name,omitempty"`
	APIKind              string                   `json:"api_kind,omitempty"`
	APIFamily            string                   `json:"api_family,omitempty"`
	NativeModel          string                   `json:"native_model,omitempty"`
	ResponseID           string                   `json:"response_id,omitempty"`
	ConsumerContinuation unified.ContinuationMode `json:"consumer_continuation,omitempty"`
	InternalContinuation unified.ContinuationMode `json:"internal_continuation,omitempty"`
	Transport            unified.TransportKind    `json:"transport,omitempty"`
	Extensions           unified.Extensions       `json:"extensions,omitempty"`
}

func NewProviderContinuation(identity ProviderIdentity, responseID string, extensions unified.Extensions) ProviderContinuation {
	return ProviderContinuation{
		ProviderName: identity.ProviderName,
		APIKind:      identity.APIKind,
		APIFamily:    identity.APIFamily,
		NativeModel:  identity.NativeModel,
		ResponseID:   responseID,
		Extensions:   extensions,
	}
}

func NewProviderContinuationFromRoute(identity ProviderIdentity, responseID string, route unified.RouteEvent, execution unified.ProviderExecutionEvent, extensions unified.Extensions) ProviderContinuation {
	continuation := NewProviderContinuation(identity, responseID, extensions)
	continuation.ConsumerContinuation = route.ConsumerContinuation
	continuation.InternalContinuation = firstContinuationMode(execution.InternalContinuation, route.InternalContinuation)
	continuation.Transport = firstTransportKind(execution.Transport, route.Transport)
	return continuation
}

func (c ProviderContinuation) Matches(identity ProviderIdentity) bool {
	if identity.ProviderName != "" && c.ProviderName != "" && identity.ProviderName != c.ProviderName {
		return false
	}
	if identity.APIKind != "" && c.APIKind != "" && !apiKindMatches(identity.APIKind, c.APIKind) {
		return false
	}
	if identity.APIFamily != "" && c.APIFamily != "" && !apiKindMatches(identity.APIFamily, c.APIFamily) {
		return false
	}
	if identity.NativeModel != "" && c.NativeModel != "" && identity.NativeModel != c.NativeModel {
		return false
	}
	return true
}

func (c ProviderContinuation) SupportsPublicPreviousResponseID() bool {
	return c.ConsumerContinuation == unified.ContinuationPreviousResponseID
}

func ContinuationAtHead(tree *Tree, branch BranchID, identity ProviderIdentity) (ProviderContinuation, bool, error) {
	path, err := tree.Path(branch)
	if err != nil {
		return ProviderContinuation{}, false, err
	}
	for i := len(path) - 1; i >= 0; i-- {
		var continuations []ProviderContinuation
		switch ev := path[i].Payload.(type) {
		case AssistantTurnEvent:
			continuations = ev.Continuations
		case *AssistantTurnEvent:
			continuations = ev.Continuations
		}
		for _, continuation := range continuations {
			if continuation.ResponseID != "" && continuation.Matches(identity) {
				return continuation, true, nil
			}
		}
	}
	return ProviderContinuation{}, false, nil
}

func ContinuationAtBranchHead(tree *Tree, branch BranchID, identity ProviderIdentity) (ProviderContinuation, bool, error) {
	if tree == nil {
		return ProviderContinuation{}, false, fmt.Errorf("conversation: tree is nil")
	}
	head, ok := tree.Head(branch)
	if !ok {
		return ProviderContinuation{}, false, fmt.Errorf("conversation: branch %q not found", branch)
	}
	if head == "" {
		return ProviderContinuation{}, false, nil
	}
	node, ok := tree.Node(head)
	if !ok {
		return ProviderContinuation{}, false, fmt.Errorf("conversation: node %q not found", head)
	}
	var continuations []ProviderContinuation
	switch ev := node.Payload.(type) {
	case AssistantTurnEvent:
		continuations = ev.Continuations
	case *AssistantTurnEvent:
		continuations = ev.Continuations
	}
	for _, continuation := range continuations {
		if continuation.ResponseID != "" && continuation.Matches(identity) {
			return continuation, true, nil
		}
	}
	return ProviderContinuation{}, false, nil
}

func isResponsesKind(kind string) bool {
	kind = strings.ToLower(strings.TrimSpace(kind))
	return kind == "responses" || strings.HasSuffix(kind, ".responses") || strings.Contains(kind, "responses")
}

func isOpenAIResponsesKind(kind string, providerName string) bool {
	kind = strings.ToLower(strings.TrimSpace(kind))
	providerName = strings.ToLower(strings.TrimSpace(providerName))
	if kind == "openai.responses" {
		return true
	}
	if kind == "responses" && strings.Contains(providerName, "openai") {
		return true
	}
	return false
}

func apiKindMatches(a, b string) bool {
	a = strings.ToLower(strings.TrimSpace(a))
	b = strings.ToLower(strings.TrimSpace(b))
	return a == b || (isResponsesKind(a) && isResponsesKind(b))
}

func firstContinuationMode(a, b unified.ContinuationMode) unified.ContinuationMode {
	if a != "" {
		return a
	}
	return b
}

func firstTransportKind(a, b unified.TransportKind) unified.TransportKind {
	if a != "" {
		return a
	}
	return b
}
