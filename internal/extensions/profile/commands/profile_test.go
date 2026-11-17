// SPDX-License-Identifier: Apache-2.0

package commands

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/profile/api"
)

// memService is an in-memory ProfileService with the real validation.
type memService struct {
	p   api.Profile
	sug api.Suggestion
}

func (m *memService) Get(context.Context) (api.Profile, error) { return m.p, nil }
func (m *memService) Update(_ context.Context, in api.UpdateInput) (api.Profile, error) {
	p := in.Profile()
	if err := p.Validate(); err != nil {
		return api.Profile{}, err
	}
	m.p = p
	return p, nil
}
func (m *memService) Suggest(context.Context) (api.Suggestion, error) { return m.sug, nil }
func (m *memService) GuideGet(context.Context) (api.GuideState, error) {
	return api.GuideState{}, nil
}
func (m *memService) GuideComplete(context.Context, api.GuideCompleteInput) (api.GuideState, error) {
	return api.GuideState{}, nil
}
func (m *memService) GuideReset(context.Context, api.GuideResetInput) (api.GuideState, error) {
	return api.GuideState{}, nil
}
func (m *memService) RelayGet(context.Context) (api.RelayState, error) { return api.RelayState{}, nil }
func (m *memService) RelaySet(_ context.Context, in api.RelaySetInput) (api.RelayState, error) {
	return api.RelayState{Enabled: in.Enabled}, nil
}

func run(t *testing.T, svc *memService, stdin string, args ...string) (string, error) {
	t.Helper()
	prev := connect
	connect = func() (api.ProfileService, func(), error) { return svc, func() {}, nil }
	t.Cleanup(func() { connect = prev })
	cmd := NewProfileCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetArgs(args)
	cmd.SetContext(context.Background())
	err := cmd.Execute()
	return out.String(), err
}

var suggestion = api.Suggestion{
	ID: "jdoe", FirstName: "Jane", LastName: "Doe",
	Emails: []api.EmailCandidate{
		{Email: "jdoe@acme.example", Source: "office"},
		{Email: "jane.doe@example.com", Source: "git"},
	},
}

func TestBareProfileShowsThePlaceholder(t *testing.T) {
	out, err := run(t, &memService{p: api.Placeholder()}, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"john-doe", "John Doe", "placeholder", "katyoti profile init"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}

func TestInitProposesTheSuggestionAndSavesOnConfirm(t *testing.T) {
	svc := &memService{p: api.Placeholder(), sug: suggestion}
	// keep handle, first, last; pick email 2; confirm.
	out, err := run(t, svc, "\n\n\n2\n\n", "init")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	want := api.Profile{ID: "jdoe", FirstName: "Jane", LastName: "Doe", Email: "jane.doe@example.com", Source: api.SourceLocal}
	if svc.p != want {
		t.Fatalf("saved %+v, want %+v\n%s", svc.p, want, out)
	}
}

func TestInitDeclinedSavesNothing(t *testing.T) {
	svc := &memService{p: api.Placeholder(), sug: suggestion}
	out, err := run(t, svc, "\n\n\n\nn\n", "init")
	if err != nil {
		t.Fatal(err)
	}
	if svc.p != api.Placeholder() || !strings.Contains(out, "Nothing saved") {
		t.Fatalf("declined init changed the profile: %+v\n%s", svc.p, out)
	}
}

func TestInitReasksAnInvalidHandle(t *testing.T) {
	svc := &memService{p: api.Placeholder(), sug: api.Suggestion{}}
	out, err := run(t, svc, "Chris@x\nchris\nC\nS\nc@io.example\ny\n", "init")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if svc.p.ID != "chris" || !strings.Contains(out, "must be lowercase") {
		t.Fatalf("saved %+v\n%s", svc.p, out)
	}
}

func TestInitYesAcceptsSuggestionWithoutPrompting(t *testing.T) {
	svc := &memService{p: api.Placeholder(), sug: suggestion}
	if _, err := run(t, svc, "", "init", "--yes"); err != nil {
		t.Fatal(err)
	}
	if svc.p.ID != "jdoe" || svc.p.Email != "jdoe@acme.example" {
		t.Fatalf("saved %+v", svc.p)
	}
}

func TestSetFromPlaceholderDoesNotKeepJohnDoe(t *testing.T) {
	svc := &memService{p: api.Placeholder()}
	if _, err := run(t, svc, "", "set", "--id", "jdoe"); err == nil {
		t.Fatalf("set --id alone from the placeholder saved %+v (John Doe's email carried over)", svc.p)
	}
	if _, err := run(t, svc, "", "set", "--id", "jdoe", "--email", "c@io.example"); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, svc, "", "set", "--first-name", "Jane"); err != nil {
		t.Fatal(err)
	}
	if svc.p.ID != "jdoe" || svc.p.Email != "c@io.example" || svc.p.FirstName != "Jane" {
		t.Fatalf("got %+v", svc.p)
	}
}

func TestSetKeepsTheLanguageAndSetsIt(t *testing.T) {
	svc := &memService{p: api.Profile{ID: "jdoe", Email: "c@example.net", Source: api.SourceLocal, Language: "fr"}}
	if out, err := run(t, svc, "", "set", "--first-name", "Jane"); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if svc.p.Language != "fr" || svc.p.FirstName != "Jane" {
		t.Fatalf("after set --first-name profile = %+v, want the language kept", svc.p)
	}
	if out, err := run(t, svc, "", "set", "--language", "en"); err != nil || !strings.Contains(out, "language: en") {
		t.Fatalf("set --language en: %v\n%s", err, out)
	}
	if _, err := run(t, svc, "", "set", "--language", "de"); err == nil {
		t.Fatal("set --language de accepted")
	}
}
