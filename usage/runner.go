package usage

import (
	"strings"

	"github.com/codewandler/agentsdk/conversation"
	"github.com/codewandler/agentsdk/runner"
)

const DefaultTransportSource = "llmadapter"

type RouteState struct {
	Provider string
	Model    string
}

func NewRouteState(identity conversation.ProviderIdentity) RouteState {
	return RouteState{
		Provider: identity.ProviderName,
		Model:    identity.NativeModel,
	}
}

func (s *RouteState) Apply(identity conversation.ProviderIdentity) {
	if s == nil {
		return
	}
	s.Provider = identity.ProviderName
	s.Model = identity.NativeModel
}

type RunnerEventOptions struct {
	TurnID         string
	SessionID      string
	ConversationID string
	BranchID       string
	FallbackModel  string
	RouteState     RouteState
	Labels         map[string]string
	Source         string
}

func FromRunnerEvent(ev runner.UsageEvent, opts RunnerEventOptions) Record {
	provider, model := ProviderAndModel(ev.ProviderIdentity, opts.RouteState, firstNonEmpty(ev.Model, opts.FallbackModel))
	rec := FromUnified(ev.Usage, Dims{
		Provider:       provider,
		Model:          model,
		TurnID:         opts.TurnID,
		SessionID:      opts.SessionID,
		ConversationID: opts.ConversationID,
		BranchID:       opts.BranchID,
		Labels:         cloneLabels(opts.Labels),
	})
	rec.Source = firstNonEmpty(opts.Source, DefaultTransportSource)
	return rec
}

func ProviderAndModel(identity conversation.ProviderIdentity, route RouteState, fallbackModel string) (string, string) {
	provider := identity.ProviderName
	model := identity.NativeModel
	if provider == "" {
		provider = route.Provider
	}
	if model == "" {
		model = route.Model
	}
	if model == "" {
		model = fallbackModel
	}
	if provider == "" && len(model) > 0 && model[0] != '/' {
		parts := strings.SplitN(model, "/", 2)
		if len(parts) == 2 {
			provider, model = parts[0], parts[1]
		}
	}
	if provider != "" && strings.HasPrefix(model, provider+"/") {
		model = strings.TrimPrefix(model, provider+"/")
	}
	return provider, model
}

func (t *Tracker) AggregateTurn(turnID string) Record {
	if t == nil {
		return Record{}
	}
	return Merge(t.Filter(ByTurnID(turnID), ExcludeEstimates())...)
}

func cloneLabels(labels map[string]string) map[string]string {
	if len(labels) == 0 {
		return nil
	}
	out := make(map[string]string, len(labels))
	for key, value := range labels {
		out[key] = value
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
