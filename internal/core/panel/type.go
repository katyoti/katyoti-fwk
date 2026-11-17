// SPDX-License-Identifier: Apache-2.0

package panel

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// TypeDecl is a panel type: the body of a `panel-type`
// contribution - what a panel of that type shows. Any pack adds its own as a
// file, embed/panel-types/<name>.yaml, read as the kind declares
// (system:/contribution-type/panel-type): no code, in the pack or in the GUI.
//
// Its page tree is named by URI (a contributed page template) or given
// inline in the page DSL, never both - the same choice as a part of the one
// answer; the panel's data is of DataType; Inputs are what a
// person may send it; Retention how long an ended panel is kept; Keep what of
// it stays on the object it worked on once it ends.
type TypeDecl struct {
	Label       string         `json:"label,omitempty" yaml:"label,omitempty" jsonschema:"description=The name shown for a panel of this type"`
	Description string         `json:"description,omitempty" yaml:"description,omitempty" jsonschema:"description=What a panel of this type shows"`
	Page        string         `json:"page,omitempty" yaml:"page,omitempty" jsonschema:"description=The page tree a panel of this type draws named by its URI (a contributed page template); or tree"`
	Tree        map[string]any `json:"tree,omitempty" yaml:"tree,omitempty" jsonschema:"description=The page tree inline in the page DSL; or page"`
	DataType    string         `json:"dataType,omitempty" yaml:"dataType,omitempty" jsonschema:"description=URI of the data-type of a panel's data"`
	Inputs      []string       `json:"inputs,omitempty" yaml:"inputs,omitempty" jsonschema:"description=What a person may send a panel of this type: submit or action or message or cancel"`
	Retention   string         `json:"retention,omitempty" yaml:"retention,omitempty" jsonschema:"description=How long an ended panel is kept before it is cleaned: a number of days (7d) or a Go duration (36h); empty = as ended runs"`
	Keep        string         `json:"keep,omitempty" yaml:"keep,omitempty" jsonschema:"description=What stays on the object the panel worked on once it ends: none (the default) or outcome (its final state and the person's inputs) or full,enum=none|outcome|full"`
}

// What a panel type keeps once a panel ends.
const (
	KeepNone    = "none"
	KeepOutcome = "outcome"
	KeepFull    = "full"
)

// Validate refuses a type with both a page URI and an inline tree or neither,
// an input it does not know, a retention it cannot read and a keep it does
// not know.
func (d TypeDecl) Validate() error {
	if (d.Page == "") == (d.Tree == nil) {
		return fmt.Errorf("panel type: a page URI or an inline tree, one of them")
	}
	for _, in := range d.Inputs {
		switch InputKind(in) {
		case InputSubmit, InputAction, InputMessage, InputCancel:
		default:
			return fmt.Errorf("panel type: unknown input %q", in)
		}
	}
	if _, err := d.RetentionDuration(); err != nil {
		return err
	}
	switch d.Keep {
	case "", KeepNone, KeepOutcome, KeepFull:
	default:
		return fmt.Errorf("panel type: keep is none or outcome or full, not %q", d.Keep)
	}
	return nil
}

// RetentionDuration reads Retention: 0 when empty (the default, as ended
// runs), N days for `Nd`, else a Go duration.
func (d TypeDecl) RetentionDuration() (time.Duration, error) {
	r := strings.TrimSpace(d.Retention)
	if r == "" {
		return 0, nil
	}
	if n, ok := strings.CutSuffix(r, "d"); ok {
		days, err := strconv.Atoi(n)
		if err != nil || days < 0 {
			return 0, fmt.Errorf("panel type: retention %q: a number of days", d.Retention)
		}
		return time.Duration(days) * 24 * time.Hour, nil
	}
	v, err := time.ParseDuration(r)
	if err != nil || v < 0 {
		return 0, fmt.Errorf("panel type: retention %q: a number of days (7d) or a duration (36h)", d.Retention)
	}
	return v, nil
}

// KeepOrDefault is what the type keeps, none when it says nothing.
func (d TypeDecl) KeepOrDefault() string {
	if d.Keep == "" {
		return KeepNone
	}
	return d.Keep
}
