package agentcontext

import (
	"context"
	"time"

	"github.com/codewandler/llmadapter/unified"
)

// ProviderKey is the stable identity of one context provider in a manager.
type ProviderKey string

// FragmentKey is the stable identity of one rendered context fragment.
type FragmentKey string

// Preference tells providers whether the caller prefers changed fragments or a
// full refresh. Providers may ignore the hint when they need to.
type Preference string

const (
	PreferChanges Preference = "changes"
	PreferFull    Preference = "full"
)

// RenderReason describes why the context manager is rendering context.
type RenderReason string

const (
	RenderInitial           RenderReason = "initial"
	RenderTurn              RenderReason = "turn"
	RenderToolFollowup      RenderReason = "tool_followup"
	RenderResume            RenderReason = "resume"
	RenderCompaction        RenderReason = "compaction"
	RenderBranchSwitch      RenderReason = "branch_switch"
	RenderForcedFullRefresh RenderReason = "forced_full_refresh"
)

// FragmentAuthority classifies who owns the rendered fragment content.
type FragmentAuthority string

const (
	AuthorityDeveloper FragmentAuthority = "developer"
	AuthorityUser      FragmentAuthority = "user"
	AuthorityTool      FragmentAuthority = "tool"
)

// CacheScope describes how broadly a model/provider cache hint may be reused.
type CacheScope string

const (
	CacheNone   CacheScope = "none"
	CacheTurn   CacheScope = "turn"
	CacheBranch CacheScope = "branch"
	CacheThread CacheScope = "thread"
)

// CachePolicy carries cache hints for rendered context fragments.
type CachePolicy struct {
	Stable bool
	MaxAge time.Duration
	Scope  CacheScope
}

// ContextFragment is one model-facing piece of contextual content.
type ContextFragment struct {
	Key         FragmentKey
	Role        unified.Role
	StartMarker string
	EndMarker   string
	Content     string
	Fingerprint string
	Authority   FragmentAuthority
	CachePolicy CachePolicy
}

// ProviderSnapshot stores provider-owned opaque state captured with a render.
type ProviderSnapshot struct {
	Fingerprint string
	Data        []byte
}

// ProviderContext is the full response from a provider for one render request.
type ProviderContext struct {
	Fragments   []ContextFragment
	Snapshot    *ProviderSnapshot
	Fingerprint string
}

// Request is the input passed to context providers during context rendering.
type Request struct {
	ThreadID     string
	BranchID     string
	TurnID       string
	HarnessState any
	Preference   Preference
	Previous     *ProviderRenderRecord
	TokenBudget  int
	Reason       RenderReason
}

// Provider contributes context fragments to a manager render.
type Provider interface {
	Key() ProviderKey
	GetContext(context.Context, Request) (ProviderContext, error)
}

// FingerprintingProvider can cheaply report whether its state changed before
// the manager asks it to build full context.
type FingerprintingProvider interface {
	Provider
	StateFingerprint(context.Context, Request) (fingerprint string, ok bool, err error)
}
