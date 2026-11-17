// SPDX-License-Identifier: Apache-2.0

package api

import "fmt"

// SizeBudgetError is the refusal of a write that would leave an object's file
// over its type's `max_size_bytes`. It is an ErrBudgetExceeded (errors.Is) and
// carries the numbers, so a writer that can shorten what it writes (a
// snapshot, a summary) knows by how much.
type SizeBudgetError struct {
	Verb  string // the refused verb (ObjectCreate, ObjectUpdate, ObjectTransition)
	ID    string // the object's id
	Type  string // its type
	Bytes int    // the file's size as the write would leave it
	Max   int    // the type's max_size_bytes
}

func (e *SizeBudgetError) Error() string {
	return fmt.Sprintf("%s: %s: object %q: its file would be %d bytes, over the %d bytes type %q allows (max_size_bytes)",
		ErrBudgetExceeded.Error(), e.Verb, e.ID, e.Bytes, e.Max, e.Type)
}

// Unwrap makes the refusal an ErrBudgetExceeded.
func (e *SizeBudgetError) Unwrap() error { return ErrBudgetExceeded }

// Over is how many bytes the file is over its budget.
func (e *SizeBudgetError) Over() int { return e.Bytes - e.Max }
