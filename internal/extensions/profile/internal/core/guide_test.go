// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/extensions/profile/api"
)

// clock returns a Now that advances one minute per call from a fixed instant.
func clock() func() time.Time {
	t := time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC)
	return func() time.Time { t = t.Add(time.Minute); return t }
}

func recipes(g api.GuideState) []string {
	out := make([]string, 0, len(g.Completed))
	for _, c := range g.Completed {
		out = append(out, c.Recipe+"/"+c.How)
	}
	return out
}

func TestUpdateAfterGuideCompleteKeepsTheCompletion(t *testing.T) {
	ctx := context.Background()
	s := New(Options{Dir: t.TempDir(), Now: clock()})
	if _, err := s.GuideComplete(ctx, api.GuideCompleteInput{Recipe: "add-a-capability", How: api.GuideChecked}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Update(ctx, api.UpdateInput{ID: "jdoe", Email: "c@example.net"}); err != nil {
		t.Fatal(err)
	}
	g, err := s.GuideGet(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := recipes(g); len(got) != 1 || got[0] != "add-a-capability/checked" {
		t.Fatalf("after Update the guide section is %v, want the completion kept", got)
	}
	if p, _ := s.Get(ctx); p.ID != "jdoe" {
		t.Fatalf("profile = %+v", p)
	}
}

func TestGuideCompleteBeforeAProfileKeepsThePlaceholder(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s := New(Options{Dir: dir, Now: clock()})
	if _, err := s.GuideComplete(ctx, api.GuideCompleteInput{Recipe: "welcome", How: api.GuideSkipped}); err != nil {
		t.Fatal(err)
	}
	p, err := s.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if p != api.Placeholder() {
		t.Fatalf("a guide-only file made profile %+v, want the placeholder", p)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "Profile.yaml"))
	if strings.Contains(string(raw), "id:") || strings.Contains(string(raw), "email:") {
		t.Fatalf("guide-only file carries empty profile fields:\n%s", raw)
	}
}

func TestGuideCompleteReplacesTheEntryOfTheSameRecipe(t *testing.T) {
	ctx := context.Background()
	s := New(Options{Dir: t.TempDir(), Now: clock()})
	uri := "ktw-univers:/guide-recipe/add-a-capability"
	for _, in := range []api.GuideCompleteInput{
		{Recipe: uri, How: api.GuideSeen},
		{Recipe: "open-an-object", How: api.GuideSkipped},
		{Recipe: " " + uri + " ", How: api.GuideChecked},
	} {
		if _, err := s.GuideComplete(ctx, in); err != nil {
			t.Fatal(err)
		}
	}
	g, err := s.GuideGet(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"open-an-object/skipped", uri + "/checked"}
	if got := recipes(g); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("completed = %v, want %v", got, want)
	}
	if g.Completed[1].At != "2026-09-30T10:03:00Z" {
		t.Fatalf("at = %q, want the later completion's stamp", g.Completed[1].At)
	}
}

func TestGuideCompleteRefusesAnInvalidInput(t *testing.T) {
	dir := t.TempDir()
	s := New(Options{Dir: dir, OnChange: func(context.Context, api.Profile) { t.Error("OnChange on a refused completion") }})
	for _, in := range []api.GuideCompleteInput{
		{Recipe: "add-a-capability", How: "done"},
		{Recipe: "add-a-capability", How: ""},
		{Recipe: "  ", How: api.GuideSeen},
	} {
		if _, err := s.GuideComplete(context.Background(), in); err == nil {
			t.Errorf("GuideComplete(%+v) accepted", in)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "Profile.yaml")); !os.IsNotExist(err) {
		t.Fatalf("a refused completion wrote the file: %v", err)
	}
}

func TestGuideResetOneThenAll(t *testing.T) {
	ctx := context.Background()
	s := New(Options{Dir: t.TempDir(), Now: clock()})
	for _, r := range []string{"a", "b", "c"} {
		if _, err := s.GuideComplete(ctx, api.GuideCompleteInput{Recipe: r, How: api.GuideSeen}); err != nil {
			t.Fatal(err)
		}
	}
	g, err := s.GuideReset(ctx, api.GuideResetInput{Recipe: "b"})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(recipes(g), ","); got != "a/seen,c/seen" {
		t.Fatalf("after reset b: %s", got)
	}
	if g, err = s.GuideReset(ctx, api.GuideResetInput{}); err != nil {
		t.Fatal(err)
	}
	if g.Completed == nil || len(g.Completed) != 0 {
		t.Fatalf("after reset all: %+v", g)
	}
	if g, _ = s.GuideGet(ctx); len(g.Completed) != 0 {
		t.Fatalf("reset all not saved: %+v", g)
	}
}

func TestEveryWriteAnnouncesTheChange(t *testing.T) {
	ctx := context.Background()
	var ids []string
	s := New(Options{Dir: t.TempDir(), Now: clock(), OnChange: func(_ context.Context, p api.Profile) { ids = append(ids, p.ID) }})

	if _, err := s.GuideComplete(ctx, api.GuideCompleteInput{Recipe: "a", How: api.GuideSeen}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Update(ctx, api.UpdateInput{ID: "jdoe", Email: "j@example.org"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GuideComplete(ctx, api.GuideCompleteInput{Recipe: "b", How: api.GuideChecked}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GuideReset(ctx, api.GuideResetInput{Recipe: "a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GuideReset(ctx, api.GuideResetInput{Recipe: "never-completed"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GuideReset(ctx, api.GuideResetInput{}); err != nil {
		t.Fatal(err)
	}
	want := "john-doe,jdoe,jdoe,jdoe,jdoe" // the no-op reset writes nothing and announces nothing
	if got := strings.Join(ids, ","); got != want {
		t.Fatalf("OnChange ids = %s, want %s", got, want)
	}
}

func TestWritesKeepASectionThisBuildDoesNotKnow(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	seed := "version: 1\nid: jdoe\nemail: j@example.org\nsource: local\nfuture:\n  flag: true\n"
	if err := os.WriteFile(filepath.Join(dir, "Profile.yaml"), []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}
	s := New(Options{Dir: dir, Now: clock()})
	if _, err := s.GuideComplete(ctx, api.GuideCompleteInput{Recipe: "a", How: api.GuideSeen}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Update(ctx, api.UpdateInput{ID: "jdoe", FirstName: "J", Email: "j@example.org"}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "Profile.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"future:", "flag: true", "guide:", "recipe: a", "how: seen", "first_name: J"} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("Profile.yaml lacks %q:\n%s", want, raw)
		}
	}
}
