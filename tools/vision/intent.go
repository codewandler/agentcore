package vision

import (
	"strings"

	"github.com/codewandler/agentsdk/tool"
)

func visionIntent() tool.TypedToolOption[Params] {
	return tool.WithDeclareIntent(func(_ tool.Ctx, p Params) (tool.Intent, error) {
		var ops []tool.IntentOperation
		for _, action := range p.Actions {
			for _, img := range action.Images {
				cat := "file"
				locality := "workspace"
				if strings.HasPrefix(img, "http://") || strings.HasPrefix(img, "https://") {
					cat = "url"
					locality = "network"
				} else if strings.HasPrefix(img, "data:") {
					cat = "file"
					locality = "workspace"
				}
				ops = append(ops, tool.IntentOperation{
					Resource:  tool.IntentResource{Category: cat, Value: img, Locality: locality},
					Operation: "read",
					Certain:   true,
				})
			}
		}
		return tool.Intent{
			Tool:       "vision",
			ToolClass:  "vision",
			Confidence: "high",
			Operations: ops,
			Behaviors:  []string{"filesystem_read"},
		}, nil
	})
}
