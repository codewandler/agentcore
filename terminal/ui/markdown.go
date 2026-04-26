package ui

import (
	"io"
	"regexp"
	"strings"
)

var ansiOnlyLineRE = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// Renderer renders Markdown for terminal output.
type Renderer func(string) string

// NewMarkdownRendererForWriter returns the default Markdown renderer.
//
// The first implementation intentionally avoids a terminal rendering dependency.
// Applications can inject a richer renderer through NewStepDisplayWithRenderer.
func NewMarkdownRendererForWriter(io.Writer) Renderer {
	return func(s string) string { return strings.TrimRight(s, "\n") }
}

// TrimOuterRenderedBlankLines removes visually blank leading and trailing lines.
func TrimOuterRenderedBlankLines(s string) string {
	if s == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	start := 0
	for start < len(lines) && IsVisuallyBlankRenderedLine(lines[start]) {
		start++
	}
	end := len(lines)
	for end > start && IsVisuallyBlankRenderedLine(lines[end-1]) {
		end--
	}
	if start >= end {
		return ""
	}
	return strings.Join(lines[start:end], "\n")
}

// IsVisuallyBlankRenderedLine reports whether a rendered line is blank after
// stripping ANSI SGR codes.
func IsVisuallyBlankRenderedLine(s string) bool {
	s = ansiOnlyLineRE.ReplaceAllString(s, "")
	return strings.TrimSpace(s) == ""
}
