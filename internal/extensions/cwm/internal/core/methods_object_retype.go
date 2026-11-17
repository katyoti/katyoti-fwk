// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"fmt"
	"sort"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// ObjectRetype changes an object's type keeping its id, its file and its
// edges (the object-levels study's E7). The capability groups it
// carries stay in force - an instance group counts as the type's would - so
// what a retype could lose is the old type's own fields: while the object
// holds some the new type does not shape the same way, it is refused, a data
// conversion (a migration step) moving them first. The level never changes:
// a project stays a project, a module a module, so the id rule holds.
func (s *Service) ObjectRetype(ctx stdctx.Context, in api.ObjectRetypeInput) (api.ObjectRetypeOutput, error) {
	if strings.TrimSpace(in.ID) == "" || strings.TrimSpace(in.Type) == "" {
		return api.ObjectRetypeOutput{}, fmt.Errorf("%w: ObjectRetype: id and type required", api.ErrInvalidInput)
	}
	obj, module, rev, err := s.fullObjectRev(ctx, in.ID)
	if err != nil {
		return api.ObjectRetypeOutput{}, err
	}
	previous := obj.Type
	if previous == in.Type {
		return api.ObjectRetypeOutput{Object: obj, Previous: previous}, nil
	}
	to, ok := s.lookupObjectType(in.Type)
	if !ok {
		return api.ObjectRetypeOutput{}, fmt.Errorf("%w: ObjectRetype: %s is no declared type", api.ErrInvalidInput, in.Type)
	}
	from, _ := s.lookupObjectType(previous)
	if from.Level() != to.Level() {
		return api.ObjectRetypeOutput{}, fmt.Errorf("%w: ObjectRetype: %s is a %s type, %s a %s one - a retype keeps the level (project or module)",
			api.ErrInvalidInput, previous, levelWord(from.Level()), in.Type, levelWord(to.Level()))
	}
	if len(obj.Fields) > 0 && (to.Fields == "" || from.Fields != to.Fields) {
		keys := make([]string, 0, len(obj.Fields))
		for k := range obj.Fields {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		return api.ObjectRetypeOutput{}, fmt.Errorf("%w: ObjectRetype: %s holds the fields of %s (%s) that %s does not shape the same way - convert them first (a migration step), then retype",
			api.ErrInvalidInput, in.ID, previous, strings.Join(keys, ", "), in.Type)
	}
	obj.Type = in.Type
	out, err := s.ObjectUpdate(ctx, api.ObjectUpdateInput{Module: module, Object: obj, Revision: rev})
	if err != nil {
		return api.ObjectRetypeOutput{}, err
	}
	return api.ObjectRetypeOutput{Object: out.Object, Previous: previous}, nil
}

func levelWord(level string) string {
	if level == "" {
		return "content"
	}
	return level
}
