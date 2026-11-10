// Package toolhost is the registry a desktop shell uses to list and call tools.
package toolhost

import (
	"encoding/json"
	"fmt"
)

// Option is one choice in a select field.
type Option struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// Field describes one option a tool exposes to the shell.
type Field struct {
	Key     string   `json:"key"`
	Label   string   `json:"label"`
	Type    string   `json:"type"`            // text, textarea, number, bool, select
	Scope   string   `json:"scope,omitempty"` // "file" is per input; empty is shared
	Default any      `json:"default"`
	Min     *float64 `json:"min,omitempty"`
	Max     *float64 `json:"max,omitempty"`
	Step    *float64 `json:"step,omitempty"`
	Help    string   `json:"help,omitempty"`
	Options []Option `json:"options,omitempty"`
}

// Meta is what the shell lists in the sidebar.
type Meta struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Panel       string  `json:"panel"`
	Schema      []Field `json:"schema"`
}

// Tool is one feature. Call dispatches a named action with a JSON payload.
type Tool interface {
	Meta() Meta
	Call(action string, payload json.RawMessage) (any, error)
}

// Registry holds tools in registration order.
type Registry struct {
	order []string
	byID  map[string]Tool
}

// New registers tools. Duplicate ids panic; that is a programming error.
func New(tools ...Tool) *Registry {
	r := &Registry{byID: map[string]Tool{}}
	for _, tool := range tools {
		id := tool.Meta().ID
		if id == "" {
			panic("tool missing id")
		}
		if _, ok := r.byID[id]; ok {
			panic("duplicate tool " + id)
		}
		r.order = append(r.order, id)
		r.byID[id] = tool
	}
	return r
}

// List returns metadata for every registered tool.
func (r *Registry) List() []Meta {
	out := make([]Meta, 0, len(r.order))
	for _, id := range r.order {
		out = append(out, r.byID[id].Meta())
	}
	return out
}

// Call runs one action and returns JSON bytes.
func (r *Registry) Call(id, action string, payload json.RawMessage) (json.RawMessage, error) {
	tool, ok := r.byID[id]
	if !ok {
		return nil, fmt.Errorf("未知工具：%s", id)
	}
	if len(payload) == 0 {
		payload = []byte("{}")
	}
	result, err := tool.Call(action, payload)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("结果无法编码")
	}
	return raw, nil
}
