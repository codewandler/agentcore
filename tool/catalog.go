package tool

import (
	"fmt"
	"path"
	"sort"
	"strings"
)

// Catalog stores tools by name for app-level selection.
//
// A Catalog is intended to be populated during setup and then read during
// request handling. Concurrent calls to Register are not safe.
type Catalog struct {
	tools map[string]Tool
	order []string
}

func NewCatalog(tools ...Tool) (*Catalog, error) {
	c := &Catalog{tools: map[string]Tool{}}
	if err := c.Register(tools...); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Catalog) Register(tools ...Tool) error {
	if c.tools == nil {
		c.tools = map[string]Tool{}
	}
	for _, t := range tools {
		if t == nil {
			continue
		}
		name := t.Name()
		if name == "" {
			return fmt.Errorf("tool: name is required")
		}
		if _, exists := c.tools[name]; exists {
			return fmt.Errorf("tool: duplicate tool %q", name)
		}
		c.tools[name] = t
		c.order = append(c.order, name)
	}
	return nil
}

func (c *Catalog) All() []Tool {
	if c == nil {
		return nil
	}
	out := make([]Tool, 0, len(c.order))
	for _, name := range c.order {
		out = append(out, c.tools[name])
	}
	return out
}

func (c *Catalog) Select(names []string) ([]Tool, error) {
	if c == nil {
		return nil, nil
	}
	if len(names) == 0 {
		return c.All(), nil
	}
	var out []Tool
	seen := map[string]bool{}
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		t, ok := c.tools[name]
		if ok {
			if !seen[name] {
				out = append(out, t)
				seen[name] = true
			}
			continue
		}
		if !hasPatternMeta(name) {
			return nil, fmt.Errorf("tool: %q not found", name)
		}
		var matched bool
		for _, toolName := range c.order {
			ok, err := path.Match(name, toolName)
			if err != nil {
				return nil, fmt.Errorf("tool: invalid pattern %q: %w", name, err)
			}
			if ok {
				matched = true
				if !seen[toolName] {
					out = append(out, c.tools[toolName])
					seen[toolName] = true
				}
			}
		}
		if !matched {
			return nil, fmt.Errorf("tool: pattern %q matched no tools", name)
		}
	}
	return out, nil
}

func (c *Catalog) Names() []string {
	if c == nil {
		return nil
	}
	names := append([]string(nil), c.order...)
	sort.Strings(names)
	return names
}

func hasPatternMeta(value string) bool {
	return strings.ContainsAny(value, "*?[")
}
