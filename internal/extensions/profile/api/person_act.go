// SPDX-License-Identifier: Apache-2.0

package api

import (
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/core/cause"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// PERSON ACTS: no check reads "is a person" on its own any more.
// An act only a person may do names its permission code and asks PersonAct,
// the one place that decides it: today a person's act is one whose actor is a
// person (the universe's person holds every code until grants exist, decision
// A4); tomorrow the same call asks iam's grants, with no caller changing. The
// proof the act requires stays the relay's (person.AcceptRelayed). An
// architecture guard (internal/test/arch, persons_acts_test.go) refuses a new
// bare person-kind test outside iam.

// PersonAct refuses an act only a person may do when its actor is not one:
// the error names the act's code, so the refusal says what was asked.
func PersonAct(by cwmapi.Actor, code PermissionCode) error {
	if err := ValidateCode(code); err != nil {
		return err
	}
	if IsPerson(by) {
		return nil
	}
	who := by.Kind
	if by.ID != "" {
		who += " " + by.ID
	}
	if who == "" {
		who = "no actor"
	}
	return fmt.Errorf("%w: %s is a person's act: %s cannot do it", cwmapi.ErrInvalidInput, code, who)
}

// IsPerson is whether an actor is a person: iam's one reading of an actor's
// kind, for what records who acted (a person's answer, a person's step) rather
// than what they may do - an act asks PersonAct.
func IsPerson(by cwmapi.Actor) bool { return by.Kind == cause.KindPerson }
