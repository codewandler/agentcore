package ui

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode"

	md "github.com/codewandler/agentsdk/markdown"
)

type State int

const (
	StateIdle State = iota
	StateReasoning
	StateText
)

// StepDisplay manages streamed output for one model/tool step.
type StepDisplay struct {
	w           io.Writer
	state       State
	buffer      *md.Buffer
	render      Renderer
	rendered    bool
	atLineStart bool
}

func NewStepDisplay(w io.Writer) *StepDisplay {
	return NewStepDisplayWithRenderer(w, NewMarkdownRendererForWriter(w))
}

func NewStepDisplayWithRenderer(w io.Writer, renderer Renderer) *StepDisplay {
	if renderer == nil {
		renderer = func(s string) string { return s }
	}
	d := &StepDisplay{w: w, render: renderer, atLineStart: true}
	d.buffer = md.NewBuffer(func(blocks []md.Block) {
		for _, block := range blocks {
			d.writeRenderedMarkdown(block.Markdown)
		}
	})
	return d
}

func (d *StepDisplay) WriteReasoning(chunk string) {
	if d.state == StateIdle {
		fmt.Fprint(d.w, Dim)
		d.state = StateReasoning
	}
	fmt.Fprint(d.w, chunk)
}

func (d *StepDisplay) WriteText(chunk string) {
	if d.state == StateReasoning {
		fmt.Fprintf(d.w, "%s\n\n", Reset)
	}
	if d.state != StateText {
		d.state = StateText
	}
	d.writeTextChunk(chunk)
}

func (d *StepDisplay) PrintToolCall(name string, args map[string]any) {
	switch d.state {
	case StateReasoning:
		fmt.Fprintf(d.w, "%s\n", Reset)
	case StateText:
		_ = d.buffer.Flush()
		fmt.Fprint(d.w, "\n")
	}
	d.state = StateIdle
	d.rendered = false
	fmt.Fprintf(d.w, "\n%s> tool: %s%s\n", BrightYellow, name, Reset)
	if len(args) == 0 {
		fmt.Fprintf(d.w, "  %s(no args)%s\n", Dim, Reset)
		return
	}
	data, _ := json.MarshalIndent(args, "  ", "  ")
	fmt.Fprintf(d.w, "  %s%s%s\n", Dim, data, Reset)
}

func (d *StepDisplay) End() {
	switch d.state {
	case StateReasoning:
		fmt.Fprintf(d.w, "%s\n", Reset)
	case StateText:
		_ = d.buffer.Flush()
		fmt.Fprint(d.w, "\n")
		d.rendered = false
	}
	d.state = StateIdle
	d.atLineStart = true
}

func (d *StepDisplay) writeRenderedMarkdown(markdown string) {
	rendered := d.render(markdown)
	if rendered == "" {
		return
	}
	if d.rendered {
		fmt.Fprint(d.w, "\n\n")
	}
	fmt.Fprint(d.w, rendered)
	d.rendered = true
}

func (d *StepDisplay) writeTextChunk(chunk string) {
	for chunk != "" {
		if d.buffer.Pending() != "" || (d.atLineStart && shouldBufferMarkdownLineStart(chunk)) {
			_, _ = d.buffer.WriteString(chunk)
			if d.buffer.Pending() == "" {
				d.atLineStart = strings.HasSuffix(chunk, "\n")
			}
			return
		}
		n := len(chunk)
		if idx := strings.IndexByte(chunk, '\n'); idx >= 0 {
			n = idx + 1
		}
		part := chunk[:n]
		fmt.Fprint(d.w, part)
		d.atLineStart = strings.HasSuffix(part, "\n")
		chunk = chunk[n:]
	}
}

func shouldBufferMarkdownLineStart(s string) bool {
	if s == "" {
		return false
	}
	segment := s
	if idx := strings.IndexByte(segment, '\n'); idx >= 0 {
		segment = segment[:idx]
	}
	trimmed := strings.TrimLeft(segment, " ")
	indent := len(segment) - len(trimmed)
	if trimmed == "" {
		return indent > 0 && indent <= 3 && !strings.Contains(s, "\n")
	}
	if indent >= 4 {
		return true
	}
	switch trimmed[0] {
	case '#', '>', '|', '<', '`', '~':
		return true
	case '-', '*', '+':
		return len(trimmed) == 1 || trimmed[1] == ' ' || trimmed[1] == '\t' || trimmed[1] == trimmed[0]
	default:
		return startsOrderedList(trimmed)
	}
}

func startsOrderedList(s string) bool {
	if s == "" || !unicode.IsDigit(rune(s[0])) {
		return false
	}
	for i, r := range s {
		if !unicode.IsDigit(r) {
			return (r == '.' || r == ')') && i+1 < len(s) && (s[i+1] == ' ' || s[i+1] == '\t')
		}
	}
	return len(s) <= 9
}
