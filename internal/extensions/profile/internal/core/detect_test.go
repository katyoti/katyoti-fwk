// SPDX-License-Identifier: Apache-2.0

package core

import (
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/profile/api"
)

func TestHandleFrom(t *testing.T) {
	for in, want := range map[string]string{
		`ACME\JDOE`:         "jdoe",
		"jdoe":              "jdoe",
		"jdoe@corp.example": "jdoe",
		"Jean.Dupont":       "jean-dupont",
		"jean__dupont":      "jean-dupont",
		`DOM\Émile`:         "mile",
		"":                  "",
		`DOM\agent01`:       "", // reserved prefix: the user types a handle
		"...":               "",
	} {
		if got := handleFrom(in); got != want {
			t.Errorf("handleFrom(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSplitDisplayName(t *testing.T) {
	for in, want := range map[string][2]string{
		"Jane DOE":            {"Jane", "Doe"},
		"DE LA FONTAINE Jean": {"Jean", "De La Fontaine"},
		"Jean-Pierre LE-GALL": {"Jean-Pierre", "Le-Gall"},
		"Jane Doe":            {"Jane", "Doe"},
		"Jane S":              {"Jane", "S"},
		"Anne Marie Dupont":   {"Anne", "Marie Dupont"},
		"JDOE":                {"JDOE", ""},
		"Madonna":             {"Madonna", ""},
		"":                    {"", ""},
		"  Jane  DOE ":        {"Jane", "Doe"},
	} {
		f, l := splitDisplayName(in)
		if f != want[0] || l != want[1] {
			t.Errorf("splitDisplayName(%q) = %q/%q, want %q/%q", in, f, l, want[0], want[1])
		}
	}
}

func TestDedupeEmails(t *testing.T) {
	got := dedupeEmails([]api.EmailCandidate{
		{Email: "JDOE@acme.example", Source: "office"},
		{Email: "jdoe@acme.example", Source: "upn"},
		{Email: "not an email", Source: "git"},
		{Email: "c@example.com", Source: "git"},
	})
	want := []api.EmailCandidate{
		{Email: "jdoe@acme.example", Source: "office"},
		{Email: "c@example.com", Source: "git"},
	}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}
