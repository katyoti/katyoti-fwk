// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/cause"
	"github.com/katyoti/katyoti-fwk/internal/extensions/profile/api"
)

func TestGetWithoutFileIsThePlaceholder(t *testing.T) {
	p, err := New(Options{Dir: t.TempDir()}).Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if p != api.Placeholder() {
		t.Fatalf("got %+v, want the John Doe placeholder", p)
	}
}

func TestUpdateSavesThenGetReadsIt(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "config")
	var changed []api.Profile
	s := New(Options{Dir: dir, OnChange: func(_ context.Context, p api.Profile) { changed = append(changed, p) }})

	in := api.UpdateInput{ID: "jdoe", FirstName: "Jane", LastName: "Doe", Email: "jane.doe@example.com"}
	saved, err := s.Update(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got != saved || got.Source != api.SourceLocal || got.ID != "jdoe" {
		t.Fatalf("got %+v, saved %+v", got, saved)
	}
	if len(changed) != 1 || changed[0] != saved {
		t.Fatalf("OnChange calls = %+v", changed)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "Profile.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"version: 1", "id: jdoe", "first_name: Jane", "source: local"} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("Profile.yaml lacks %q:\n%s", want, raw)
		}
	}
}

func TestUpdateRefusesAnInvalidProfileAndWritesNothing(t *testing.T) {
	dir := t.TempDir()
	s := New(Options{Dir: dir, OnChange: func(context.Context, api.Profile) { t.Error("OnChange on a refused update") }})
	for _, in := range []api.UpdateInput{
		{ID: "jane.doe@example.com", Email: "jane.doe@example.com"},
		{ID: "agent-smith", Email: "a@co.example"},
		{ID: "jdoe", Email: "not-an-email"},
	} {
		if _, err := s.Update(context.Background(), in); err == nil {
			t.Errorf("Update(%+v) accepted", in)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "Profile.yaml")); !os.IsNotExist(err) {
		t.Fatalf("a refused update wrote the file: %v", err)
	}
}

func TestHandWrittenFileWithoutSourceIsLocal(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Profile.yaml"), []byte("id: jdoe\nemail: j@example.org\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := New(Options{Dir: dir}).Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if p.ID != "jdoe" || p.Source != api.SourceLocal {
		t.Fatalf("got %+v", p)
	}
}

func TestLanguageIsSavedAndKeptAcrossAGuideComplete(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s := New(Options{Dir: dir, Now: clock()})
	if _, err := s.Update(ctx, api.UpdateInput{ID: "jdoe", Email: "c@example.net", Language: "fr"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GuideComplete(ctx, api.GuideCompleteInput{Recipe: "welcome", How: api.GuideSeen}); err != nil {
		t.Fatal(err)
	}
	p, err := s.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if p.Language != "fr" {
		t.Fatalf("after GuideComplete language = %q, want fr", p.Language)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "Profile.yaml"))
	if !strings.Contains(string(raw), "language: fr") {
		t.Fatalf("Profile.yaml lacks the language:\n%s", raw)
	}
	// An Update that changes the language keeps the guide section.
	if _, err := s.Update(ctx, api.UpdateInput{ID: "jdoe", Email: "c@example.net", Language: "en"}); err != nil {
		t.Fatal(err)
	}
	if p, _ := s.Get(ctx); p.Language != "en" {
		t.Fatalf("after Update language = %q, want en", p.Language)
	}
	if g, _ := s.GuideGet(ctx); len(g.Completed) != 1 {
		t.Fatalf("guide section after Update = %+v", g)
	}
	// Empty language = follow the OS: written as unset.
	if _, err := s.Update(ctx, api.UpdateInput{ID: "jdoe", Email: "c@example.net"}); err != nil {
		t.Fatal(err)
	}
	if p, _ := s.Get(ctx); p.Language != "" {
		t.Fatalf("after Update without language = %q, want empty", p.Language)
	}
}

func TestUpdateRefusesAnUnsupportedLanguage(t *testing.T) {
	dir := t.TempDir()
	s := New(Options{Dir: dir})
	if _, err := s.Update(context.Background(), api.UpdateInput{ID: "jdoe", Email: "c@example.net", Language: "de"}); err == nil {
		t.Fatal("Update with language de accepted")
	}
	if _, err := os.Stat(filepath.Join(dir, "Profile.yaml")); !os.IsNotExist(err) {
		t.Fatalf("a refused update wrote the file: %v", err)
	}
}

// Relaying is off until the person turns it on; turning it on or off from an
// agent's session is refused and writes nothing; ReadRelay reads what was set.
func TestRelayIsThePersonsSwitch(t *testing.T) {
	dir := t.TempDir()
	s := New(Options{Dir: dir})
	ctx := context.Background()
	if r, err := s.RelayGet(ctx); err != nil || r.Enabled {
		t.Fatalf("never set: %+v, %v; want off", r, err)
	}
	agent := cause.WithCause(ctx, cause.Cause{Kind: cause.KindAgent, ID: "claude-code"})
	if _, err := s.RelaySet(agent, api.RelaySetInput{Enabled: true}); err == nil {
		t.Fatal("an agent's session turned relaying on; want refused")
	}
	if ReadRelay(dir).Enabled {
		t.Fatal("a refused RelaySet wrote the switch")
	}
	person := cause.WithCause(ctx, cause.Cause{Kind: cause.KindPerson, ID: "jdoe"})
	if r, err := s.RelaySet(person, api.RelaySetInput{Enabled: true}); err != nil || !r.Enabled || r.At == "" {
		t.Fatalf("the person turned it on: %+v, %v", r, err)
	}
	if !ReadRelay(dir).Enabled {
		t.Fatal("ReadRelay: off after the person turned it on")
	}
}
