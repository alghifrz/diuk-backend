package tools

import (
	"encoding/json"
	"fmt"
	"sort"
	"sync"
)

type Registry struct {
	mu     sync.RWMutex
	tools  map[string]Tool
	quotes QuoteStore
}

func NewRegistry() *Registry {
	return &Registry{tools: map[string]Tool{}}
}

// SetQuotes exposes the booking quote store so the orchestrator can show the
// model the pending booking state at the start of every turn.
func (r *Registry) SetQuotes(store QuoteStore) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.quotes = store
}

func (r *Registry) Quotes() QuoteStore {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.quotes
}

func (r *Registry) Register(tool Tool) error {
	if tool == nil || tool.Name() == "" {
		return fmt.Errorf("%w: name is required", ErrUnknownTool)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.tools[tool.Name()]; exists {
		return fmt.Errorf("%w: %s", ErrDuplicate, tool.Name())
	}
	r.tools[tool.Name()] = tool
	return nil
}

func (r *Registry) Get(name string) (Tool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tool, ok := r.tools[name]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownTool, name)
	}
	return tool, nil
}

func (r *Registry) List() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Tool, 0, len(r.tools))
	for _, tool := range r.tools {
		out = append(out, tool)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out
}

func (r *Registry) Definitions() []Definition {
	items := r.List()
	out := make([]Definition, 0, len(items))
	for _, tool := range items {
		raw, _ := json.Marshal(tool.InputSchema())
		out = append(out, Definition{
			Name:        tool.Name(),
			Description: tool.Description(),
			Parameters:  raw,
		})
	}
	return out
}

type Definition struct {
	Name        string
	Description string
	Parameters  json.RawMessage
}
