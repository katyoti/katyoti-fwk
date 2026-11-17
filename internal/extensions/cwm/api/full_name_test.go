// SPDX-License-Identifier: Apache-2.0

package api

import (
	"errors"
	"testing"
)

const testProjectID = "00000000-0000-7000-8000-000000000001"

func TestFullNameRoundTrips(t *testing.T) {
	for _, id := range []string{"cwm.g1", "compose.CMP-010", "jobs.UC-063-AC1", "live.019ee62f-4a81-714c-ba02-9d06b78d4e6a"} {
		fn := FullName(testProjectID, id)
		p, back, err := ParseFullName(fn)
		if err != nil || p != testProjectID || back != id {
			t.Errorf("%s → %q → (%q, %q, %v)", id, fn, p, back, err)
		}
	}
}

// A BARE ID IS NOT A FULL NAME. (But `<uuid>.g1` IS one: a bare authored id is
// a module object's or the project's — its module is itself.) The in-project resolver accepts `cwm.g1`; the
// full-name parser refuses it, so a log reader can never mistake one for the
// other and resolve against the wrong project.
func TestParseFullNameRefusesWhatIsNotOne(t *testing.T) {
	for _, s := range []string{
		"cwm.g1", // bare authored id
		"not-a-uuid-at-all-but-thirty-six-chars.cwm.g1", // 36 chars, not a uuid
		testProjectID,       // a uuid alone
		testProjectID + ".", // no id
		"",
	} {
		if _, _, err := ParseFullName(s); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("%q parsed as a full name — err=%v", s, err)
		}
	}
	if IsFullName("cwm.g1") || !IsFullName(testProjectID+".cwm.g1") {
		t.Error("IsFullName disagrees with ParseFullName")
	}
}
