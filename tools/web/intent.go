package web

import (
	"net/url"
	"strings"

	"github.com/codewandler/agentsdk/tool"
)

func webFetchIntent() tool.TypedToolOption[WebFetchParams] {
	return tool.WithDeclareIntent(func(ctx tool.Ctx, p WebFetchParams) (tool.Intent, error) {
		method := strings.ToUpper(p.Method)
		if method == "" {
			method = "GET"
		}

		op := "network_read"
		behavior := "network_fetch"
		if method == "POST" || method == "PUT" || method == "PATCH" || method == "DELETE" {
			op = "network_write"
			behavior = "network_write"
		}

		locality := "network"
		if u, err := url.Parse(p.URL); err == nil && isLocalhost(u.Hostname()) {
			locality = "workspace"
		}

		return tool.Intent{
			Tool:       "web_fetch",
			ToolClass:  "network_access",
			Confidence: "high",
			Operations: []tool.IntentOperation{{
				Resource:  tool.IntentResource{Category: "url", Value: p.URL, Locality: locality},
				Operation: op,
				Certain:   true,
			}},
			Behaviors: []string{behavior},
		}, nil
	})
}

func webSearchIntent() tool.TypedToolOption[WebSearchParams] {
	return tool.WithDeclareIntent(func(_ tool.Ctx, p WebSearchParams) (tool.Intent, error) {
		return tool.Intent{
			Tool:       "web_search",
			ToolClass:  "network_access",
			Confidence: "high",
			Operations: []tool.IntentOperation{{
				Resource:  tool.IntentResource{Category: "service", Value: "web_search", Locality: "network"},
				Operation: "network_read",
				Certain:   true,
			}},
			Behaviors: []string{"network_fetch"},
		}, nil
	})
}

func isLocalhost(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1" || host == "[::1]"
}
