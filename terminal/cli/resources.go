package cli

import (
	"fmt"
	"io/fs"

	"github.com/codewandler/agentsdk/agentdir"
	"github.com/codewandler/agentsdk/resource"
)

type Resources interface {
	Resolve(resource.DiscoveryPolicy) (agentdir.Resolution, error)
}

type ResourceFunc func(resource.DiscoveryPolicy) (agentdir.Resolution, error)

func (f ResourceFunc) Resolve(policy resource.DiscoveryPolicy) (agentdir.Resolution, error) {
	if f == nil {
		return agentdir.Resolution{}, fmt.Errorf("cli: resources are required")
	}
	return f(policy)
}

func DirResources(path string) Resources {
	return ResourceFunc(func(policy resource.DiscoveryPolicy) (agentdir.Resolution, error) {
		if path == "" {
			return agentdir.Resolution{}, fmt.Errorf("cli: resource path is required")
		}
		return agentdir.ResolveDirWithOptions(path, agentdir.ResolveOptions{Policy: policy})
	})
}

func EmbeddedResources(fsys fs.FS, root string) Resources {
	return ResourceFunc(func(resource.DiscoveryPolicy) (agentdir.Resolution, error) {
		return agentdir.ResolveFS(fsys, root)
	})
}

func ResolvedResources(resolution agentdir.Resolution) Resources {
	return ResourceFunc(func(resource.DiscoveryPolicy) (agentdir.Resolution, error) {
		return resolution, nil
	})
}
