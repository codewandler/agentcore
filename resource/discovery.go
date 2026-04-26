package resource

import "context"

type Root struct {
	URI   string
	Label string
	Scope Scope
}

type Request struct {
	Roots  []Root
	Policy DiscoveryPolicy
}

type Discoverer interface {
	Name() string
	Discover(context.Context, Request) ([]ContributionBundle, error)
}

type DiscovererFunc struct {
	DiscovererName string
	Fn             func(context.Context, Request) ([]ContributionBundle, error)
}

func (d DiscovererFunc) Name() string { return d.DiscovererName }

func (d DiscovererFunc) Discover(ctx context.Context, req Request) ([]ContributionBundle, error) {
	if d.Fn == nil {
		return nil, nil
	}
	return d.Fn(ctx, req)
}
