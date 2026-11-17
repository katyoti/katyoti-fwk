// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/usermsg"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// mutations/ slot — declarative single-field writes on CWM OBJECTS, so a type
// or capability can bind e.g. an "Archive" button without authoring a task or
// a script. One method:
//
//	<provider>:mutations/set-field   {node_id | id, field, value}
//
// Fields, in the object vocabulary (the cut, 2026-09-26):
//   - `<capability>.<field>` — one field of a capability group
//     (`module.decisions`);
//   - `fields.<key>` — one of the type's own fields;
//   - the object core: `name`, `description`, `icon`, `tags`, `body` (its
//     prose).
//
// A null value deletes the field. Identity and structure (id, type,
// parent) are NOT mutable here — moves and re-typing have their own
// guarded verbs. The input key stays `node_id` because that is what every
// action sender passes; `id` is accepted too.

// setFieldInput is the decoded payload for mutations/set-field.
type setFieldInput struct {
	NodeID string `json:"node_id"`
	ID     string `json:"id"`
	Field  string `json:"field"`
	Value  any    `json:"value"`
}

// setFieldSchema is the static input schema Describe returns — flat
// enough for auto-generated parameter dialogs.
var setFieldSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "node_id": {"type": "string", "description": "Target object id"},
    "field":   {"type": "string", "description": "<capability>.<field> | fields.<key> | name | description | icon | tags | body"},
    "value":   {"description": "New value; type depends on the field"}
  },
  "required": ["node_id", "field"]
}`)

func (s *Service) describeMutation(_ context.Context, uri mbapi.ToolURI) (actionsapi.DescribeOutput, error) {
	if uri.Method != "set-field" {
		return actionsapi.DescribeOutput{}, usermsg.Wrap(fmt.Errorf("unknown mutation %q (v1 ships set-field)", uri.Method), "actions.unknownAction", map[string]any{"uri": uri.String()})
	}
	return actionsapi.DescribeOutput{
		URI:         uri.String(),
		Label:       "Set field",
		Description: "Set one field on a CWM object (<capability>.<field>, fields.<key>, or the core name / description / icon / tags / body)",
		InputSchema: setFieldSchema,
		IsAsync:     false,
	}, nil
}

func (s *Service) runMutation(ctx context.Context, uri mbapi.ToolURI, input json.RawMessage) (actionsapi.RunOutput, error) {
	if uri.Method != "set-field" {
		return actionsapi.RunOutput{}, usermsg.Wrap(fmt.Errorf("unknown mutation %q (v1 ships set-field)", uri.Method), "actions.unknownAction", map[string]any{"uri": uri.String()})
	}
	var in setFieldInput
	if len(input) > 0 {
		if err := json.Unmarshal(input, &in); err != nil {
			return actionsapi.RunOutput{}, fmt.Errorf("decode set-field input: %w", err)
		}
	}
	if in.NodeID == "" {
		in.NodeID = in.ID
	}
	if in.NodeID == "" || in.Field == "" {
		return actionsapi.RunOutput{}, fmt.Errorf("set-field: node_id (or id) and field are required")
	}
	// Validated BEFORE any substrate access: a malformed request never
	// resolves a service or reads an object.
	field, err := normalizeSetField(in.Field, in.Value)
	if err != nil {
		return actionsapi.RunOutput{}, err
	}
	in.Field = field
	if s.app == nil {
		return actionsapi.RunOutput{}, fmt.Errorf("ActionService.Run: no Application available (factory was constructed without one)")
	}
	svc, err := application.AppUseService[cwmapi.CWMService](s.app)
	if err != nil {
		return actionsapi.RunOutput{}, fmt.Errorf("resolve CWMService: %w", err)
	}
	module, err := cwmapi.ModuleOfObjectID(in.NodeID)
	if err != nil {
		return actionsapi.RunOutput{}, fmt.Errorf("%w: set-field: %q is not an object id", cwmapi.ErrInvalidInput, in.NodeID)
	}
	got, err := svc.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: in.NodeID})
	if err != nil {
		return actionsapi.RunOutput{}, err
	}
	obj := got.Object
	if err := setObjectField(&obj, in.Field, in.Value); err != nil {
		return actionsapi.RunOutput{}, err
	}
	if _, err := svc.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: module, Object: obj}); err != nil {
		return actionsapi.RunOutput{}, err
	}
	value, err := json.Marshal(map[string]any{"node_id": obj.ID, "field": in.Field})
	if err != nil {
		return actionsapi.RunOutput{}, fmt.Errorf("encode set-field output: %w", err)
	}
	return actionsapi.RunOutput{URI: uri.String(), Value: value}, nil
}

// normalizeSetField checks a field and its value without touching anything:
// the object's own fields (name, description, icon, tags, body), a field of
// its type (`fields.<key>`), or a capability's field (`<capability>.<field>`). The parent is moved by its own
// verb, a state by a transition.
func normalizeSetField(field string, value any) (string, error) {
	switch field {
	case "name", "description", "icon":
		if _, err := stringValue(field, value); err != nil {
			return "", err
		}
		return field, nil
	case "tags":
		if _, err := stringListValue(value); err != nil {
			return "", err
		}
		return field, nil
	case "parent":
		return "", usermsg.Wrap(fmt.Errorf("set-field: parent is structure, not mutable here — moving has its own verb"), "actions.fieldNotMutable", map[string]any{"field": "parent"})
	}
	if field == "body" {
		if _, err := stringValue(field, value); err != nil {
			return "", err
		}
		return field, nil
	}
	group, key, ok := strings.Cut(field, ".")
	if !ok || group == "" || key == "" {
		return "", usermsg.Wrap(fmt.Errorf("set-field: field %q is not mutable here — use name, description, icon, tags, body, fields.<key> or <capability>.<field>", field), "actions.fieldNotMutable", map[string]any{"field": field})
	}
	if group == "container" || group == "content" {
		return "", usermsg.Wrap(fmt.Errorf("set-field: %s — the object's name, description, parent, tags and body are its own fields now (name, description, tags, body)", field), "actions.fieldNotMutable", map[string]any{"field": field})
	}
	if group == "lifecycle" {
		return "", usermsg.Wrap(fmt.Errorf("set-field: %s changes only through a transition (ObjectTransition), which checks the workflow and its gate", field), "actions.transitionOnly", map[string]any{"field": field})
	}
	return field, nil
}

// setObjectField writes one (normalized) field of an object.
func setObjectField(obj *cwmapi.Object, field string, value any) error {
	switch field {
	case "name":
		obj.Name, _ = value.(string)
		return nil
	case "description":
		obj.Description, _ = value.(string)
		return nil
	case "icon":
		obj.Icon, _ = value.(string)
		return nil
	case "tags":
		tags, err := stringListValue(value)
		if err != nil {
			return err
		}
		obj.Tags = tags
		return nil
	case "body":
		obj.Body, _ = value.(string)
		return nil
	}
	group, key, _ := strings.Cut(field, ".")
	if group == "fields" {
		fields := map[string]any{}
		for k, v := range obj.Fields {
			fields[k] = v
		}
		if value == nil {
			delete(fields, key)
		} else {
			fields[key] = value
		}
		obj.Fields = fields
		return nil
	}
	if obj.Capabilities == nil {
		obj.Capabilities = map[string]map[string]any{}
	}
	g := obj.Capabilities[group]
	if g == nil {
		g = map[string]any{}
	}
	if value == nil {
		delete(g, key)
	} else {
		g[key] = value
	}
	obj.Capabilities[group] = g
	return nil
}

func stringValue(field string, v any) (string, error) {
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("set-field: %s requires a string value, got %T", field, v)
	}
	return s, nil
}

func stringListValue(v any) ([]string, error) {
	raw, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("set-field: tags requires a string list, got %T", v)
	}
	out := make([]string, 0, len(raw))
	for _, e := range raw {
		s, ok := e.(string)
		if !ok {
			return nil, fmt.Errorf("set-field: tags entries must be strings, got %T", e)
		}
		out = append(out, s)
	}
	return out, nil
}
