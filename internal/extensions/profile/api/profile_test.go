// SPDX-License-Identifier: Apache-2.0

package api

import (
	"reflect"
	"testing"

	corereflect "github.com/katyoti/katyoti-fwk/internal/core/reflect"
)

func TestValidateHandle(t *testing.T) {
	for _, ok := range []string{"jdoe", "john-doe", "a", "x2", "jean-luc-2"} {
		if err := ValidateHandle(ok); err != nil {
			t.Errorf("ValidateHandle(%q) = %v, want ok", ok, err)
		}
	}
	for _, bad := range []string{
		"", "JDoe", "j.doe", "j doe", "-x", "x-", "a--b",
		"jane.doe@example.com",        // the email is never the id
		"agent", "agentur", "agent-x", // reserved: inbox reads these as agents
		string(make([]byte, 64)),
	} {
		if err := ValidateHandle(bad); err == nil {
			t.Errorf("ValidateHandle(%q) accepted, want refused", bad)
		}
	}
}

func TestValidateEmail(t *testing.T) {
	if err := ValidateEmail("jane.doe@example.com"); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"", "nope", "Chris <c@example.com>", "c@"} {
		if err := ValidateEmail(bad); err == nil {
			t.Errorf("ValidateEmail(%q) accepted", bad)
		}
	}
}

func TestUpdateInputProfileTrimsAndMarksLocal(t *testing.T) {
	p := UpdateInput{ID: " jdoe ", FirstName: " Jane", LastName: "Doe ", Email: " c@example.com "}.Profile()
	want := Profile{ID: "jdoe", FirstName: "Jane", LastName: "Doe", Email: "c@example.com", Source: SourceLocal}
	if p != want {
		t.Fatalf("got %+v, want %+v", p, want)
	}
}

func TestPlaceholderIsJohnDoe(t *testing.T) {
	p := Placeholder()
	if p.ID != "john-doe" || p.DisplayName() != "John Doe" || p.Source != SourcePlaceholder {
		t.Fatalf("placeholder = %+v", p)
	}
	if err := p.Validate(); err != nil {
		t.Fatalf("placeholder must itself be a valid profile: %v", err)
	}
}

// Every method must stay KTSP-shaped, or it silently drops off the wire.
func TestProfileService_KTSPSurface(t *testing.T) {
	got := map[string]bool{}
	for _, m := range corereflect.DiscoverMethods(reflect.TypeOf((*ProfileService)(nil)).Elem()) {
		got[m.Name] = true
	}
	for name := range ProfileServiceDef.Methods {
		if !got[name] {
			t.Errorf("ProfileService.%s is declared but not KTSP-exposable", name)
		}
	}
}

func TestValidateLanguage(t *testing.T) {
	for _, ok := range []string{"", "en", "fr"} {
		if err := ValidateLanguage(ok); err != nil {
			t.Errorf("ValidateLanguage(%q) = %v, want ok", ok, err)
		}
	}
	for _, bad := range []string{"de", "FR", "fr-FR", "english"} {
		if err := ValidateLanguage(bad); err == nil {
			t.Errorf("ValidateLanguage(%q) accepted, want refused", bad)
		}
		p := UpdateInput{ID: "jdoe", Email: "c@example.com", Language: bad}.Profile()
		if err := p.Validate(); err == nil {
			t.Errorf("Profile with language %q validated, want refused", bad)
		}
	}
	if p := (UpdateInput{ID: "jdoe", Email: "c@example.com", Language: " fr "}).Profile(); p.Language != "fr" || p.Validate() != nil {
		t.Errorf("trimmed language profile = %+v", p)
	}
}
