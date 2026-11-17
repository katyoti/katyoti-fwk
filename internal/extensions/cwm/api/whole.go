// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"fmt"
)

// WHOLE OBJECTS. ObjectList and ObjectTree answer from CWM's index, and an
// index copy carries no fields and no body: the index is lean by design, the
// file holds them. A list or a tree gives the SHAPE — ids, types, parents,
// capabilities, edges; whatever a reader takes from an object's fields or its
// prose body comes from the object itself, read with ObjectGet. These helpers
// are that read, written once for every extension.

// WholeObjects returns objs with the objects of the given types read whole
// through ObjectGet. No types means every object.
func WholeObjects(ctx context.Context, cwm CWMService, objs []Object, types ...string) ([]Object, error) {
	return NewWholeReader(ctx, cwm, types...).All(objs)
}

// WholeReader reads objects whole once each — a reader meeting the same object
// in a list and again in a tree reads its file once.
type WholeReader struct {
	ctx   context.Context
	cwm   CWMService
	types map[string]bool // empty = every type
	read  map[string]Object
}

// NewWholeReader reads the objects of the given types whole; no types means
// every object.
func NewWholeReader(ctx context.Context, cwm CWMService, types ...string) *WholeReader {
	r := &WholeReader{ctx: ctx, cwm: cwm, read: map[string]Object{}}
	if len(types) > 0 {
		r.types = map[string]bool{}
		for _, t := range types {
			r.types[t] = true
		}
	}
	return r
}

// All returns objs, each of a selected type read whole.
func (r *WholeReader) All(objs []Object) ([]Object, error) {
	out := make([]Object, len(objs))
	for i, o := range objs {
		w, err := r.One(o)
		if err != nil {
			return nil, err
		}
		out[i] = w
	}
	return out, nil
}

// One returns o read whole when its type is selected, else o as given.
func (r *WholeReader) One(o Object) (Object, error) {
	if r.types != nil && !r.types[o.Type] {
		return o, nil
	}
	if w, ok := r.read[o.ID]; ok {
		return w, nil
	}
	// The module is a fallback for the path only: CWM finds the object by its
	// id in the index.
	module, _ := ModuleOfObjectID(o.ID)
	got, err := r.cwm.ObjectGet(r.ctx, ObjectGetInput{Module: module, ID: o.ID})
	if err != nil {
		return Object{}, fmt.Errorf("read %s %s: %w", o.Type, o.ID, err)
	}
	r.read[o.ID] = got.Object
	return got.Object, nil
}
