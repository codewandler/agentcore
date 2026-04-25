package conversation

import "github.com/codewandler/llmadapter/unified"

type ProjectionInput struct {
	Tree                    *Tree
	Branch                  BranchID
	ProviderIdentity        ProviderIdentity
	Messages                []unified.Message
	PendingMessages         []unified.Message
	Extensions              unified.Extensions
	AllowNativeContinuation bool
}

type ProjectionResult struct {
	Messages   []unified.Message
	Extensions unified.Extensions
}

type ProjectionPolicy interface {
	Project(input ProjectionInput) (ProjectionResult, error)
}

type ProjectionPolicyFunc func(ProjectionInput) (ProjectionResult, error)

func (f ProjectionPolicyFunc) Project(input ProjectionInput) (ProjectionResult, error) {
	return f(input)
}

func DefaultProjectionPolicy() ProjectionPolicy {
	return ProjectionPolicyFunc(defaultProject)
}

func NewMessageBudgetProjectionPolicy(maxMessages int) ProjectionPolicy {
	return NewBudgetProjectionPolicy(DefaultProjectionPolicy(), BudgetOptions{MaxMessages: maxMessages})
}

type BudgetOptions struct {
	MaxMessages int
}

func NewBudgetProjectionPolicy(base ProjectionPolicy, opts BudgetOptions) ProjectionPolicy {
	if base == nil {
		base = DefaultProjectionPolicy()
	}
	return ProjectionPolicyFunc(func(input ProjectionInput) (ProjectionResult, error) {
		result, err := base.Project(input)
		if err != nil {
			return ProjectionResult{}, err
		}
		if opts.MaxMessages <= 0 || len(result.Messages) <= opts.MaxMessages {
			return result, nil
		}
		result.Messages = append([]unified.Message(nil), result.Messages[len(result.Messages)-opts.MaxMessages:]...)
		return result, nil
	})
}

func defaultProject(input ProjectionInput) (ProjectionResult, error) {
	extensions := cloneExtensions(input.Extensions)
	pendingMessages := append([]unified.Message(nil), input.PendingMessages...)
	if input.AllowNativeContinuation && SupportsPreviousResponseID(input.ProviderIdentity) && !extensions.Has(unified.ExtOpenAIPreviousResponseID) {
		continuation, ok, err := ContinuationAtBranchHead(input.Tree, input.Branch, input.ProviderIdentity)
		if err != nil {
			return ProjectionResult{}, err
		}
		if ok {
			extensions = mergeExtensions(continuation.Extensions, input.Extensions)
			if !extensions.Has(unified.ExtOpenAIPreviousResponseID) {
				if err := extensions.Set(unified.ExtOpenAIPreviousResponseID, continuation.ResponseID); err != nil {
					return ProjectionResult{}, err
				}
			}
			return ProjectionResult{Messages: pendingMessages, Extensions: extensions}, nil
		}
	}
	messages := append([]unified.Message(nil), input.Messages...)
	messages = append(messages, pendingMessages...)
	return ProjectionResult{Messages: messages, Extensions: extensions}, nil
}

func cloneExtensions(in unified.Extensions) unified.Extensions {
	var out unified.Extensions
	for _, key := range in.Keys() {
		_ = out.SetRaw(key, in.Raw(key))
	}
	return out
}

func mergeExtensions(base unified.Extensions, overlay unified.Extensions) unified.Extensions {
	out := cloneExtensions(base)
	for _, key := range overlay.Keys() {
		_ = out.SetRaw(key, overlay.Raw(key))
	}
	return out
}
