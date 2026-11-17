// SPDX-License-Identifier: Apache-2.0

package core

import (
	"fmt"
	"os"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// THE SIZE BUDGET. A type's `max_size_bytes` caps its object FILE: the bytes
// on disk, in the spelling the object takes (`.cwm.yaml`, or `.cwm.md` with its
// prose), measured as the write path encodes them — through the file already
// there, so a kept comment counts too. Bytes, not characters: the budget is
// what the file costs to hold and to read whole, and "é" is two of them.
//
// A write over budget is refused in warn mode as in error mode, like the
// description budget: warn is for the migration gap — a type nobody declared —
// and an overrun is not that; the type and its cap are declared, and the file
// breaks it. Nothing is checked when validation is off. An object read over budget
// carries the error (sizeProblem), and it is repaired one write at a time — a
// write that does not make an object already over budget bigger goes through.

// checkSizeBudget refuses a write that would leave obj's file at path over its
// type's max_size_bytes.
func (s *Service) checkSizeBudget(path string, obj api.Object, verb string) error {
	if s.typeValidationMode() == api.TypeValidationOff {
		return nil
	}
	t, ok := s.lookupObjectType(obj.Type)
	if !ok || t.MaxSizeBytes <= 0 {
		return nil
	}
	data, err := encodeObjectFile(path, obj)
	if err != nil {
		return nil // the write itself reports what cannot be encoded
	}
	n := len(data)
	if n <= t.MaxSizeBytes {
		return nil
	}
	if st, err := os.Stat(path); err == nil && st.Size() > int64(t.MaxSizeBytes) && int64(n) <= st.Size() {
		return nil // already over budget, and this write does not grow it
	}
	return &api.SizeBudgetError{Verb: verb, ID: obj.ID, Type: obj.Type, Bytes: n, Max: t.MaxSizeBytes}
}

// sizeProblem is the budget as a read sees it: the stored file over its type's
// max_size_bytes.
func (s *Service) sizeProblem(path string, obj api.Object) []api.ObjectDiagnostic {
	t, ok := s.lookupObjectType(obj.Type)
	if !ok || t.MaxSizeBytes <= 0 {
		return nil
	}
	st, err := os.Stat(path)
	if err != nil || st.Size() <= int64(t.MaxSizeBytes) {
		return nil
	}
	label := fmt.Sprintf("the file is %d bytes, over the %d bytes its type allows", st.Size(), t.MaxSizeBytes)
	return []api.ObjectDiagnostic{{Code: api.DiagnosticInvalid, Label: label,
		Description: fmt.Sprintf("%s: %s (type %q, max_size_bytes)", api.ErrBudgetExceeded.Error(), label, obj.Type)}}
}
