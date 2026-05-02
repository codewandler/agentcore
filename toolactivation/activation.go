// Package toolactivation manages the set of tools visible to an agent at runtime.
package toolactivation

import (
	"path/filepath"
	"sync"

	"github.com/codewandler/agentsdk/tool"
)

// ContextKey is the tool context Extra key used to pass mutable tool activation
// state to tool-management projections.
const ContextKey = "agentsdk.activation_state"

// State manages which tools are active or inactive.
type State interface {
	AllTools() []tool.Tool
	ActiveTools() []tool.Tool
	Activate(patterns ...string) []string
	Deactivate(patterns ...string) []string
}

// Manager is an in-memory tool activation registry.
type Manager struct {
	mu        sync.RWMutex
	allTools  []tool.Tool
	activeSet map[string]bool
}

// New creates a Manager with all provided tools initially active.
func New(tools ...tool.Tool) *Manager {
	m := &Manager{
		activeSet: make(map[string]bool),
	}
	_ = m.Register(tools...)
	return m
}

// Register adds tools and marks newly registered tools active.
func (m *Manager) Register(tools ...tool.Tool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.activeSet == nil {
		m.activeSet = make(map[string]bool)
	}
	seen := make(map[string]bool, len(m.allTools))
	for _, t := range m.allTools {
		seen[t.Name()] = true
	}
	for _, t := range tools {
		if t == nil {
			continue
		}
		name := t.Name()
		if seen[name] {
			continue
		}
		m.allTools = append(m.allTools, t)
		m.activeSet[name] = true
		seen[name] = true
	}
	return nil
}

// AllTools returns all registered tools.
func (m *Manager) AllTools() []tool.Tool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]tool.Tool(nil), m.allTools...)
}

// ActiveTools returns currently active tools.
func (m *Manager) ActiveTools() []tool.Tool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	active := make([]tool.Tool, 0, len(m.allTools))
	for _, t := range m.allTools {
		if m.activeSet[t.Name()] {
			active = append(active, t)
		}
	}
	return active
}

// Activate makes tools matching patterns active and returns names activated by this call.
func (m *Manager) Activate(patterns ...string) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var activated []string
	for _, t := range m.allTools {
		for _, pattern := range patterns {
			if matchesPattern(t.Name(), pattern) && !m.activeSet[t.Name()] {
				m.activeSet[t.Name()] = true
				activated = append(activated, t.Name())
				break
			}
		}
	}
	return activated
}

// Deactivate makes tools matching patterns inactive and returns names deactivated by this call.
func (m *Manager) Deactivate(patterns ...string) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var deactivated []string
	for _, t := range m.allTools {
		for _, pattern := range patterns {
			if matchesPattern(t.Name(), pattern) && m.activeSet[t.Name()] {
				m.activeSet[t.Name()] = false
				deactivated = append(deactivated, t.Name())
				break
			}
		}
	}
	return deactivated
}

func matchesPattern(name, pattern string) bool {
	matched, _ := filepath.Match(pattern, name)
	return matched
}

var _ State = (*Manager)(nil)
