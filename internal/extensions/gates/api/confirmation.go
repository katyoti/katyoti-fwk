// SPDX-License-Identifier: Apache-2.0

package api

import "strings"

// A PERSON'S REQUIREMENT IS CONFIRMED BY A HUMAN TASK - a
// requirement met by a person's confirmation is asked as an `approve` human
// task whose `satisfies` names it: the object, the gate, the requirement and
// the wait it belongs to (when the refused move began waiting), so one
// object's answer never confirms another's and an answer given for an earlier
// wait - one since revoked - never confirms a new one. The asker records the
// confirmation the approving answer gives (Confirm, by whoever answered); the
// gate reads it there.

// confirmationPrefix opens a confirmation item.
const confirmationPrefix = "gate:"

// ConfirmationItem is what a human task confirming one requirement of a gate
// on an object satisfies: gate:<object>/<gate>/<requirement>@<since>.
func ConfirmationItem(object, gate, requirement, since string) string {
	return confirmationPrefix + object + "/" + gate + "/" + requirement + "@" + since
}

// Confirmation is a confirmation item read back.
type Confirmation struct {
	Object      string
	Gate        string
	Requirement string
	// Since is when the wait it belongs to began.
	Since string
}

// ParseConfirmationItem reads a confirmation item; false for any other item a
// human task satisfies (a verification, a step).
func ParseConfirmationItem(item string) (Confirmation, bool) {
	rest, ok := strings.CutPrefix(item, confirmationPrefix)
	if !ok {
		return Confirmation{}, false
	}
	what, since, ok := strings.Cut(rest, "@")
	if !ok || since == "" {
		return Confirmation{}, false
	}
	parts := strings.Split(what, "/")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return Confirmation{}, false
	}
	return Confirmation{Object: parts[0], Gate: parts[1], Requirement: parts[2], Since: since}, true
}
