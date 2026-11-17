// SPDX-License-Identifier: Apache-2.0

package chatpanel

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/terminal"
)

// The chat artifact's own ops (modules/display/streaming-protocol.md
// « Patches »), applied here - the one place they are applied: the bridge
// keeps each artifact's value and sends it whole, every chat draws it as it
// is.

// applyOps applies ops in order to tree and answers the new tree: an op
// naming a target node id (and no path) is resolved through the tree's
// id → pointer map, rebuilt after each op so an op may target a node the
// previous one added. An op that cannot apply is skipped and said in errs.
func applyOps(tree any, ops []terminal.UIPatchOp) (any, []error) {
	var errs []error
	for i, op := range ops {
		if op.Target != "" && op.Path == "" {
			p, ok := idLookup(tree)[op.Target]
			if !ok {
				errs = append(errs, fmt.Errorf("op[%d]: unknown target %q", i, op.Target))
				continue
			}
			op.Path = p
		}
		next, err := applyOp(tree, op)
		if err != nil {
			errs = append(errs, fmt.Errorf("op[%d] %s: %w", i, op.Kind, err))
			continue
		}
		tree = next
	}
	return tree, errs
}

// applyOp applies one op to a copy of value.
func applyOp(value any, op terminal.UIPatchOp) (any, error) {
	val, err := decodeRaw(op.Value)
	if err != nil {
		return value, err
	}
	switch op.Kind {
	case "rfc6902":
		return applyJSONPatch(value, op, val)
	case "rfc7396":
		patch := val
		if len(op.MergePatch) > 0 {
			if patch, err = decodeRaw(op.MergePatch); err != nil {
				return value, err
			}
		}
		if patch == nil && len(op.MergePatch) == 0 && len(op.Value) == 0 {
			return value, nil
		}
		// At its path, the node its target names: the documented merge
		// (streaming-protocol « Patches »); the GUI's store merges at the
		// root whatever the path.
		doc := clone(value)
		at := parsePointer(op.Path)
		cur, _ := getAt(doc, at)
		return setAt(doc, at, mergePatch(cur, patch), true)
	case "append":
		return appendAt(value, op.Path, val)
	case "replace":
		return setAt(clone(value), parsePointer(op.Path), val, true)
	case "remove":
		return removeAt(clone(value), parsePointer(op.Path))
	default:
		return value, fmt.Errorf("unknown op kind %q", op.Kind)
	}
}

// applyJSONPatch is one RFC 6902 operation; a failed one leaves value as it was.
func applyJSONPatch(value any, op terminal.UIPatchOp, val any) (any, error) {
	if op.Op == "" || op.Path == "" {
		return value, fmt.Errorf("rfc6902 needs its op and path")
	}
	doc := clone(value)
	path := parsePointer(op.Path)
	switch op.Op {
	case "add":
		return addAt(doc, path, val)
	case "remove":
		if _, ok := getAt(doc, path); !ok {
			return value, fmt.Errorf("remove: no value at %s", op.Path)
		}
		return removeAt(doc, path)
	case "replace":
		if _, ok := getAt(doc, path); !ok {
			return value, fmt.Errorf("replace: no value at %s", op.Path)
		}
		return setAt(doc, path, val, false)
	case "move", "copy":
		from := parsePointer(op.From)
		v, ok := getAt(doc, from)
		if !ok {
			return value, fmt.Errorf("%s: no value at %s", op.Op, op.From)
		}
		v = clone(v)
		if op.Op == "move" {
			var err error
			if doc, err = removeAt(doc, from); err != nil {
				return value, err
			}
		}
		return addAt(doc, path, v)
	case "test":
		v, ok := getAt(doc, path)
		if !ok || !reflect.DeepEqual(v, val) {
			return value, fmt.Errorf("test failed at %s", op.Path)
		}
		return value, nil
	default:
		return value, fmt.Errorf("unknown rfc6902 op %q", op.Op)
	}
}

// parsePointer splits a JSON Pointer (RFC 6901); a path that does not start
// with "/" is one field name, as an agent often writes ("items").
func parsePointer(p string) []string {
	if p == "" || p == "/" {
		return nil
	}
	if !strings.HasPrefix(p, "/") {
		return []string{p}
	}
	segs := strings.Split(p[1:], "/")
	for i, s := range segs {
		segs[i] = strings.ReplaceAll(strings.ReplaceAll(s, "~1", "/"), "~0", "~")
	}
	return segs
}

func getAt(doc any, path []string) (any, bool) {
	cur := doc
	for _, seg := range path {
		switch c := cur.(type) {
		case map[string]any:
			v, ok := c[seg]
			if !ok {
				return nil, false
			}
			cur = v
		case []any:
			i, err := strconv.Atoi(seg)
			if err != nil || i < 0 || i >= len(c) {
				return nil, false
			}
			cur = c[i]
		default:
			return nil, false
		}
	}
	return cur, true
}

// update walks to the parent of path - creating missing objects when create
// is set - and answers the root with the parent's child replaced by what fn
// answers from it. fn sees the child (nil and false when absent).
func update(doc any, path []string, create bool, fn func(parent any, key string) (any, error)) (any, error) {
	if len(path) == 0 {
		return fn(nil, "")
	}
	var walk func(cur any, i int) (any, error)
	walk = func(cur any, i int) (any, error) {
		if i == len(path)-1 {
			return fn(cur, path[i])
		}
		seg := path[i]
		switch c := cur.(type) {
		case map[string]any:
			child, ok := c[seg]
			if !ok || child == nil {
				if !create {
					return nil, fmt.Errorf("no value at /%s", strings.Join(path[:i+1], "/"))
				}
				child = map[string]any{}
			}
			nc, err := walk(child, i+1)
			if err != nil {
				return nil, err
			}
			c[seg] = nc
			return c, nil
		case []any:
			idx, err := strconv.Atoi(seg)
			if err != nil || idx < 0 || idx >= len(c) {
				return nil, fmt.Errorf("no element %q", seg)
			}
			nc, err := walk(c[idx], i+1)
			if err != nil {
				return nil, err
			}
			c[idx] = nc
			return c, nil
		case nil:
			if !create {
				return nil, fmt.Errorf("no value at /%s", strings.Join(path[:i], "/"))
			}
			return walk(map[string]any{}, i)
		default:
			return nil, fmt.Errorf("/%s is no object or array", strings.Join(path[:i], "/"))
		}
	}
	return walk(doc, 0)
}

// setAt sets path to val (the root itself for an empty path).
func setAt(doc any, path []string, val any, create bool) (any, error) {
	if len(path) == 0 {
		return val, nil
	}
	return update(doc, path, create, func(parent any, key string) (any, error) {
		switch p := parent.(type) {
		case map[string]any:
			p[key] = val
			return p, nil
		case []any:
			i, err := strconv.Atoi(key)
			if err != nil || i < 0 || i >= len(p) {
				return nil, fmt.Errorf("no element %q", key)
			}
			p[i] = val
			return p, nil
		case nil:
			if !create {
				return nil, fmt.Errorf("no parent for %q", key)
			}
			return map[string]any{key: val}, nil
		default:
			return nil, fmt.Errorf("cannot set %q in a %T", key, parent)
		}
	})
}

// addAt is RFC 6902 add: into an object it sets, into an array it inserts
// at the index (or at the end for "-").
func addAt(doc any, path []string, val any) (any, error) {
	if len(path) == 0 {
		return val, nil
	}
	return update(doc, path, false, func(parent any, key string) (any, error) {
		switch p := parent.(type) {
		case map[string]any:
			p[key] = val
			return p, nil
		case []any:
			if key == "-" {
				return append(p, val), nil
			}
			i, err := strconv.Atoi(key)
			if err != nil || i < 0 || i > len(p) {
				return nil, fmt.Errorf("no index %q", key)
			}
			out := make([]any, 0, len(p)+1)
			out = append(out, p[:i]...)
			out = append(out, val)
			return append(out, p[i:]...), nil
		default:
			return nil, fmt.Errorf("cannot add %q to a %T", key, parent)
		}
	})
}

// removeAt deletes path (the root for an empty path: nil).
func removeAt(doc any, path []string) (any, error) {
	if len(path) == 0 {
		return nil, nil
	}
	return update(doc, path, false, func(parent any, key string) (any, error) {
		switch p := parent.(type) {
		case map[string]any:
			delete(p, key)
			return p, nil
		case []any:
			i, err := strconv.Atoi(key)
			if err != nil || i < 0 || i >= len(p) {
				return nil, fmt.Errorf("no element %q", key)
			}
			return append(p[:i:i], p[i+1:]...), nil
		default:
			return nil, fmt.Errorf("cannot remove %q from a %T", key, parent)
		}
	})
}

// appendAt pushes val onto the array at path, making it when absent.
func appendAt(value any, path string, val any) (any, error) {
	segs := parsePointer(path)
	doc := clone(value)
	if len(segs) == 0 {
		arr, ok := doc.([]any)
		if !ok {
			return value, fmt.Errorf("append at the root needs an array")
		}
		return append(arr, val), nil
	}
	cur, ok := getAt(doc, segs)
	if !ok || cur == nil {
		return setAt(doc, segs, []any{val}, true)
	}
	if arr, isArr := cur.([]any); isArr {
		return setAt(doc, segs, append(arr, val), true)
	}
	return value, fmt.Errorf("append target %s is no array", path)
}

// mergePatch is RFC 7396: objects merged, null deletes, anything else
// replaces.
func mergePatch(target, patch any) any {
	pm, ok := patch.(map[string]any)
	if !ok {
		return patch
	}
	tm, ok := target.(map[string]any)
	if !ok {
		tm = map[string]any{}
	}
	for k, v := range pm {
		if v == nil {
			delete(tm, k)
			continue
		}
		tm[k] = mergePatch(tm[k], v)
	}
	return tm
}

// idLookup maps each node id of tree to its JSON Pointer, the first one
// kept when two share an id ("/" for the root).
func idLookup(tree any) map[string]string {
	out := map[string]string{}
	var walk func(n any, at string)
	walk = func(n any, at string) {
		switch c := n.(type) {
		case []any:
			for i, v := range c {
				walk(v, at+"/"+strconv.Itoa(i))
			}
		case map[string]any:
			if id, _ := c["id"].(string); id != "" {
				if _, seen := out[id]; !seen {
					p := at
					if p == "" {
						p = "/"
					}
					out[id] = p
				}
			}
			for k, v := range c {
				if k == "id" {
					continue
				}
				walk(v, at+"/"+strings.ReplaceAll(strings.ReplaceAll(k, "~", "~0"), "/", "~1"))
			}
		}
	}
	walk(tree, "")
	return out
}

// decodeRaw reads an op's JSON value (nil when absent).
func decodeRaw(raw json.RawMessage) (any, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, fmt.Errorf("value: %w", err)
	}
	return v, nil
}

// clone deep-copies a JSON value, so a patch never changes what was sent.
func clone(v any) any {
	switch c := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(c))
		for k, x := range c {
			out[k] = clone(x)
		}
		return out
	case []any:
		out := make([]any, len(c))
		for i, x := range c {
			out[i] = clone(x)
		}
		return out
	default:
		return v
	}
}
