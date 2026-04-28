// Package markdown provides markdown parsing and rendering utilities.
//
// This package re-exports the public API from github.com/codewandler/markdown.
//
// For simple rendering, use RenderString() or RenderToWriter().
// For streaming, import stream and terminal subpackages directly:
//
//	import (
//		"github.com/codewandler/markdown/stream"
//		"github.com/codewandler/markdown/terminal"
//	)
//
//	parser := stream.NewParser()
//	renderer := terminal.NewRenderer(os.Stdout)
package markdown

import (
	"io"

	extmd "github.com/codewandler/markdown"
)

// RenderString renders a markdown string to terminal output.
// It returns the rendered output as a string.
func RenderString(markdown string) (string, error) {
	return extmd.RenderString(markdown)
}

// RenderToWriter renders a markdown string to the given writer.
func RenderToWriter(w io.Writer, markdown string) error {
	return extmd.RenderToWriter(w, markdown)
}
