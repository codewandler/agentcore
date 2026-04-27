package agent

import (
	"fmt"

	"github.com/codewandler/llmadapter/adapterconfig"
)

func pinnedConfigForSelection(cfg adapterconfig.Config, selection adapterconfig.UseCaseModelSelection, requestModel string) (adapterconfig.Config, error) {
	resolution := selection.Resolution
	if resolution.Provider == "" {
		return adapterconfig.Config{}, fmt.Errorf("agent: selected route has no provider")
	}
	if resolution.SourceAPI == "" {
		return adapterconfig.Config{}, fmt.Errorf("agent: selected route has no source api")
	}
	if resolution.ProviderAPI == "" {
		return adapterconfig.Config{}, fmt.Errorf("agent: selected route has no provider api")
	}

	provider, err := selectedProvider(cfg, selection)
	if err != nil {
		return adapterconfig.Config{}, err
	}
	model := requestModel
	if model == "" {
		model = resolution.PublicModel
	}
	if model == "" {
		model = resolution.Input
	}
	if model == "" {
		model = resolution.NativeModel
	}
	pinned := adapterconfig.Config{
		Addr:           cfg.Addr,
		HealthCooldown: cfg.HealthCooldown,
		MaxAttempts:    cfg.MaxAttempts,
		ModelDB:        cfg.ModelDB,
		Providers:      []adapterconfig.ProviderConfig{provider},
		Routes: []adapterconfig.RouteConfig{{
			SourceAPI:   resolution.SourceAPI,
			Model:       model,
			Provider:    resolution.Provider,
			ProviderAPI: resolution.ProviderAPI,
			NativeModel: resolution.NativeModel,
			Weight:      resolution.Weight,
		}},
	}
	adapterconfig.ApplyDefaults(&pinned)
	return pinned, nil
}

func selectedProvider(cfg adapterconfig.Config, selection adapterconfig.UseCaseModelSelection) (adapterconfig.ProviderConfig, error) {
	resolution := selection.Resolution
	for _, provider := range cfg.Providers {
		if provider.Name != resolution.Provider {
			continue
		}
		if resolution.ProviderType != "" && provider.Type != resolution.ProviderType {
			continue
		}
		endpoint, err := adapterconfig.ProviderEndpointConfig(provider)
		if err != nil {
			return adapterconfig.ProviderConfig{}, err
		}
		if endpoint.APIKind != resolution.ProviderAPI {
			continue
		}
		return provider, nil
	}
	return adapterconfig.ProviderConfig{}, fmt.Errorf("agent: selected provider endpoint %q %q not found", resolution.Provider, resolution.ProviderAPI)
}
