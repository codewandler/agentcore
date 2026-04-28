// Package markdown provides markdown parsing and rendering utilities.
//
// For YAML frontmatter parsing see [Parse] and [Bind] in frontmatter.go.
//
// For terminal rendering use [RenderString] or [RenderToWriter], which delegate
// to github.com/codewandler/markdown.
//
// For streaming block buffering or the stream parser, import the upstream
// subpackages directly:
//
//	import (
//		"github.com/codewandler/markdown"
//		"github.com/codewandler/markdown/stream"
//		"github.com/codewandler/markdown/terminal"
//	)
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
