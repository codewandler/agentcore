package cli

import (
	"fmt"
	"io/fs"

	"github.com/codewandler/agentsdk/agentdir"
)

type Resources interface {
	Resolve() (agentdir.Resolution, error)
}

type ResourceFunc func() (agentdir.Resolution, error)

func (f ResourceFunc) Resolve() (agentdir.Resolution, error) {
	if f == nil {
		return agentdir.Resolution{}, fmt.Errorf("cli: resources are required")
	}
	return f()
}

func DirResources(path string) Resources {
	return ResourceFunc(func() (agentdir.Resolution, error) {
		if path == "" {
			return agentdir.Resolution{}, fmt.Errorf("cli: resource path is required")
		}
		return agentdir.ResolveDir(path)
	})
}

func EmbeddedResources(fsys fs.FS, root string) Resources {
	return ResourceFunc(func() (agentdir.Resolution, error) {
		return agentdir.ResolveFS(fsys, root)
	})
}

func ResolvedResources(resolution agentdir.Resolution) Resources {
	return ResourceFunc(func() (agentdir.Resolution, error) {
		return resolution, nil
	})
}
