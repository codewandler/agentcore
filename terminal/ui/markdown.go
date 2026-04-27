package ui

import (
	"io"
	"os"
	"regexp"
	"strings"
	"sync"

	"github.com/charmbracelet/glamour"
	"golang.org/x/term"
)

var ansiOnlyLineRE = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// Renderer renders Markdown for terminal output.
type Renderer func(string) string

// NewMarkdownRendererForWriter returns the default Markdown renderer that
// converts Markdown to ANSI-styled terminal output using glamour.
//
// When the writer is connected to a terminal, glamour auto-detects the
// appropriate color style (dark/light). Otherwise it falls back to the dark
// style so that piped output still contains ANSI codes for downstream
// consumers that expect them.
func NewMarkdownRendererForWriter(w io.Writer) Renderer {
	var (
		mu sync.Mutex
		tr *glamour.TermRenderer
	)
	return func(s string) string {
		mu.Lock()
		defer mu.Unlock()
		if tr == nil {
			opts := []glamour.TermRendererOption{
				glamour.WithWordWrap(0),
			}
			if isTermWriter(w) {
				opts = append(opts, glamour.WithAutoStyle())
			} else {
				opts = append(opts, glamour.WithStandardStyle("dark"))
			}
			r, err := glamour.NewTermRenderer(opts...)
			if err != nil {
				return strings.TrimRight(s, "\n")
			}
			tr = r
		}
		out, err := tr.Render(s)
		if err != nil {
			return strings.TrimRight(s, "\n")
		}
		return TrimOuterRenderedBlankLines(out)
	}
}

// isTermWriter reports whether w is backed by a terminal file descriptor.
func isTermWriter(w io.Writer) bool {
	type fder interface{ Fd() uintptr }
	if f, ok := w.(fder); ok {
		return term.IsTerminal(int(f.Fd()))
	}
	if w == os.Stdout || w == os.Stderr {
		return term.IsTerminal(int(os.Stdout.Fd()))
	}
	return false
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
