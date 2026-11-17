// SPDX-License-Identifier: Apache-2.0

package api

import (
	"encoding/json"
	"fmt"
)

// An update that names no revision overwrites whatever is stored, an edit made
// outside CWM since the object was read included. Every surface an update
// arrives through from outside the process — the `cwm object-update` verb, the
// cwm-object-update agent tool, the KTSP method (a sidecar's client, the GUI,
// an action) — hands CWM JSON, and decoding it marks the input; a marked
// update without a revision is refused unless it says Force. A Go caller in
// the process builds the input itself and is not marked: it states its own
// condition (the revision it read when it read-modify-writes).

// UnmarshalJSON decodes the input and marks it as arrived from outside.
func (in *ObjectUpdateInput) UnmarshalJSON(b []byte) error {
	type plain ObjectUpdateInput
	var p plain
	if err := json.Unmarshal(b, &p); err != nil {
		return err
	}
	*in = ObjectUpdateInput(p)
	in.outside = true
	return nil
}

// Unconditioned refuses an update from outside that names neither the
// revision it read nor Force: it would silently overwrite a newer edit.
func (in ObjectUpdateInput) Unconditioned() error {
	if !in.outside || in.Revision != "" || in.Force {
		return nil
	}
	return fmt.Errorf("%w: ObjectUpdate of %q names no revision: read the object first and pass its revision (or --force to overwrite)",
		ErrInvalidInput, in.Object.ID)
}
