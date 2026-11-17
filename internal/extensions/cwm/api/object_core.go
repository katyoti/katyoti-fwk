// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
)

// IdentityPanel is the group id under which the pane's header saves the
// object core: a save of that "group" writes the object's Identity.
const IdentityPanel = "identity"

// FieldsPanel is the id of the section that shows the TYPE's own fields: a
// form from the type's `fields:` data-type, or its `view:` when it declares
// one, and the body as the type declares it. Its values are Object.Fields, and
// a save of that "group" writes them.
const FieldsPanel = "fields"

// Identity is the editable part of the object core — what the pane's header
// shows and edits. The parent is moved by ObjectMove, the badges are set by
// capabilities, and the body is what its type says it is (edited with the
// type's fields), so none of them is here.
type Identity struct {
	Name        string   `json:"name,omitempty"`
	Description string   `json:"description,omitempty"`
	Icon        string   `json:"icon,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

// Identity returns the object's editable core.
func (o Object) Identity() Identity {
	return Identity{Name: o.Name, Description: o.Description, Icon: o.Icon, Tags: append([]string(nil), o.Tags...)}
}

// SetIdentity writes the editable core; the body is left as it is.
func (o *Object) SetIdentity(id Identity) {
	o.Name, o.Description, o.Icon = id.Name, id.Description, id.Icon
	o.Tags = append([]string(nil), id.Tags...)
}

// Badge is one mark in an object's header: a label with an icon and a colour,
// its description as a tooltip, and the capability that set it.
type Badge struct {
	ID         string `json:"id"                   yaml:"id"                   jsonschema:"Stable id within the object"`
	Label      string `json:"label"                yaml:"label"                jsonschema:"What the badge says"`
	Icon       string `json:"icon,omitempty"       yaml:"icon,omitempty"       jsonschema:"An icon name"`
	Color      string `json:"color,omitempty"      yaml:"color,omitempty"      jsonschema:"A colour name or hex"`
	Tooltip    string `json:"tooltip,omitempty"    yaml:"tooltip,omitempty"    jsonschema:"Shown on hover"`
	Capability string `json:"capability,omitempty" yaml:"capability,omitempty" jsonschema:"The capability that set it; empty = set by a person"`
}

// FieldsJSON is the type's fields as JSON — for a caller that decodes them
// into its own type. Nil when the object has none.
func (o Object) FieldsJSON() json.RawMessage {
	if len(o.Fields) == 0 {
		return nil
	}
	b, err := json.Marshal(o.Fields)
	if err != nil {
		return nil
	}
	return b
}

// SetFieldsJSON sets the type's fields from a JSON object; empty (or not an
// object) clears them.
func (o *Object) SetFieldsJSON(raw json.RawMessage) {
	o.Fields = nil
	if m, ok := plainValue(raw).(map[string]any); ok && len(m) > 0 {
		o.Fields = m
	}
}

// plainValue turns a json.RawMessage (or any JSON-able value) into plain Go
// values — string, map[string]any, []any — the shape YAML and JSON both carry.
func plainValue(v any) any {
	var raw []byte
	switch t := v.(type) {
	case json.RawMessage:
		raw = t
	case []byte:
		raw = t
	case string, map[string]any, []any, float64, bool, nil:
		return t
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return t
		}
		raw = b
	}
	if len(raw) == 0 {
		return nil
	}
	var out any
	if json.Unmarshal(raw, &out) != nil {
		return nil
	}
	return out
}
