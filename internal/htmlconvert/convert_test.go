package htmlconvert

import (
	"testing"
)

func TestToMarkdown_ValidHTML(t *testing.T) {
	html := `<h1>Hello</h1><p>This is a <strong>test</strong>.</p>`
	result := ToMarkdown(html)
	
	// Should contain markdown-like content
	if result == "" {
		t.Error("expected non-empty result")
	}
	if result == html {
		t.Error("expected HTML to be converted to markdown")
	}
}

func TestToMarkdown_PlainText(t *testing.T) {
	text := "Just plain text"
	result := ToMarkdown(text)
	
	// Plain text should pass through
	if result != text {
		t.Errorf("expected %q, got %q", text, result)
	}
}

func TestToMarkdown_InvalidHTML_ReturnsOriginal(t *testing.T) {
	// Malformed HTML should still be handled gracefully
	html := `<div>Unclosed div`
	result := ToMarkdown(html)
	
	// Should return something (either converted or original)
	if result == "" {
		t.Error("expected non-empty result")
	}
}

func TestToMarkdown_EmptyString(t *testing.T) {
	result := ToMarkdown("")
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestToMarkdown_HTMLWithLinks(t *testing.T) {
	html := `<a href="https://example.com">Example</a>`
	result := ToMarkdown(html)
	
	// Should contain the link text
	if result == "" {
		t.Error("expected non-empty result")
	}
	// Should contain "Example" in some form
	if !contains(result, "Example") && !contains(result, "example") {
		t.Errorf("expected result to contain link text, got %q", result)
	}
}

func contains(s, substr string) bool {
	for i := 0; i < len(s)-len(substr)+1; i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
