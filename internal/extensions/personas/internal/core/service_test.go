// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"encoding/json"
	"errors"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/personas/api"
)

// stubCatalog implements mbapi.Catalog over a fixed list of
// persona entries. Used to test Resolve/List without spinning
// up the full sidecar-discovery + manifest pipeline.
type stubCatalog struct {
	entries []contribapi.Contribution
}

func (s *stubCatalog) Get(_ stdctx.Context, _ string) (*mbapi.Manifest, error) {
	return nil, mbapi.ErrSidecarNotInCatalog
}
func (s *stubCatalog) List(_ stdctx.Context) ([]*mbapi.Manifest, error) {
	return nil, nil
}
func (s *stubCatalog) Names(_ stdctx.Context) ([]string, error) { return nil, nil }
func (s *stubCatalog) GetContributionsByType(_ stdctx.Context, t string) ([]contribapi.Contribution, error) {
	var out []contribapi.Contribution
	for _, e := range s.entries {
		if e.Type == t {
			out = append(out, e)
		}
	}
	return out, nil
}
func (s *stubCatalog) GetBody(_ stdctx.Context, uri string) ([]byte, error) {
	pu, err := contribapi.ParseResourceURI(uri)
	if err != nil {
		return nil, err
	}
	for _, e := range s.entries {
		if e.Type == pu.Type && e.ID == pu.ID && e.Provider == pu.Provider {
			return e.Data(), nil
		}
	}
	return nil, mbapi.ErrSidecarNotInCatalog
}

// withCatalog stitches a stub catalog into the application
// context so the personas service can resolve it via UseService.
func withCatalog(t *testing.T, entries []mbapi.PersonaBody) stdctx.Context {
	t.Helper()
	// Build resource entries from the typed bodies.
	rentries := make([]contribapi.Contribution, 0, len(entries))
	for _, b := range entries {
		raw, _ := json.Marshal(b)
		rentries = append(rentries, contribapi.Contribution{
			Provider: "test-sidecar",
			Type:     mbapi.ResourceTypePersona,
			ID:       b.Name,
			Layers:   contribapi.InlineData(raw),
		})
	}
	app := application.New(application.Config())
	application.PutService[mbapi.Catalog](app, "default", &stubCatalog{entries: rentries})
	return application.WithApplication(stdctx.Background(), app)
}

func reviewer() mbapi.PersonaBody {
	return mbapi.PersonaBody{
		TaskBody: mbapi.TaskBody{
			Name:        "reviewer",
			Type:        "agent-loop",
			Description: "Reviews code changes.",
			Body:        "You are a code reviewer.",
			Category:    "persona",
			Types:       []string{"project", "module"},
		},
		BindMode:    "object",
		DefaultGate: "code-reviewed",
		Skills:      []string{"cwm-graph-navigation"},
	}
}

func planner() mbapi.PersonaBody {
	return mbapi.PersonaBody{
		TaskBody: mbapi.TaskBody{
			Name:     "planner",
			Type:     "agent-loop",
			Body:     "You are a planner.",
			Category: "persona",
		},
		BindMode:    "global",
		DefaultGate: "planning-complete",
	}
}

func TestResolve_HitsCatalog(t *testing.T) {
	svc := New()
	ctx := withCatalog(t, []mbapi.PersonaBody{reviewer(), planner()})

	out, err := svc.Resolve(ctx, api.ResolveInput{Name: "reviewer"})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if out.Persona.Name != "reviewer" {
		t.Errorf("Name = %q, want reviewer", out.Persona.Name)
	}
	if out.Persona.Tier != "built-in" {
		t.Errorf("Tier = %q, want built-in", out.Persona.Tier)
	}
	if out.Persona.DefaultGate != "code-reviewed" {
		t.Errorf("DefaultGate = %q, want code-reviewed", out.Persona.DefaultGate)
	}
}

func TestResolve_NotFound(t *testing.T) {
	svc := New()
	ctx := withCatalog(t, []mbapi.PersonaBody{reviewer()})
	_, err := svc.Resolve(ctx, api.ResolveInput{Name: "missing"})
	if !errors.Is(err, api.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestResolve_RequiresName(t *testing.T) {
	svc := New()
	ctx := withCatalog(t, []mbapi.PersonaBody{reviewer()})
	_, err := svc.Resolve(ctx, api.ResolveInput{})
	if !errors.Is(err, api.ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput", err)
	}
}

func TestList_FilterByCategory(t *testing.T) {
	svc := New()
	ctx := withCatalog(t, []mbapi.PersonaBody{reviewer(), planner()})

	out, err := svc.List(ctx, api.ListInput{Category: "persona"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if got, want := len(out.Personas), 2; got != want {
		t.Errorf("count = %d, want %d", got, want)
	}
}

func TestList_FilterByType(t *testing.T) {
	svc := New()
	ctx := withCatalog(t, []mbapi.PersonaBody{reviewer(), planner()})

	out, _ := svc.List(ctx, api.ListInput{Type: "project"})
	// reviewer allows project + module; planner has empty list = any type.
	// Both should match.
	if got := len(out.Personas); got != 2 {
		t.Errorf("count = %d, want 2 (reviewer matches, planner has empty=any)", got)
	}

	out, _ = svc.List(ctx, api.ListInput{Type: "task"})
	// reviewer doesn't allow task; planner (empty=any) does.
	if got := len(out.Personas); got != 1 {
		t.Errorf("filter by 'task' count = %d, want 1 (only planner)", got)
	}
	if out.Personas[0].Name != "planner" {
		t.Errorf("matched %q, want planner", out.Personas[0].Name)
	}
}

func TestSpawn_RejectsMissingTarget_BindObject(t *testing.T) {
	svc := New()
	ctx := withCatalog(t, []mbapi.PersonaBody{reviewer()})

	out, err := svc.Spawn(ctx, api.SpawnInput{Name: "reviewer"})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if out.Status != "rejected" {
		t.Errorf("Status = %q, want rejected", out.Status)
	}
	if out.Reason == "" {
		t.Error("Reason empty; want explanation about the target")
	}
}

func TestSpawn_QueuesWhenTaskServiceAbsent(t *testing.T) {
	svc := New()
	ctx := withCatalog(t, []mbapi.PersonaBody{reviewer()})

	// reviewer is bind_mode=object — we provide a target so
	// validation passes; TaskService is NOT registered on the
	// app so Spawn returns "queued".
	out, err := svc.Spawn(ctx, api.SpawnInput{
		Name:   "reviewer",
		Target: "billing.m1",
	})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	if out.Status != "queued" {
		t.Errorf("Status = %q, want queued (TaskService not in ctx)", out.Status)
	}
	if out.SpawnID == "" {
		t.Error("SpawnID empty; want allocated UUID even on queued")
	}
}

func TestSpawn_GlobalBindNoTargetNeeded(t *testing.T) {
	svc := New()
	ctx := withCatalog(t, []mbapi.PersonaBody{planner()})

	out, err := svc.Spawn(ctx, api.SpawnInput{Name: "planner"})
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	// Without a TaskService it falls through to queued. The key
	// assertion: bind_mode=global doesn't reject on missing
	// target.
	if out.Status == "rejected" {
		t.Errorf("bind_mode=global was rejected: %s", out.Reason)
	}
}

// Two-phase bootstrap protocol stubs (hub-boot-order ADR).
func (*stubCatalog) SetSchema(_ mbapi.CatalogSchemaService) error { return nil }
func (*stubCatalog) Reindex(_ stdctx.Context, _ []string) error   { return nil }
func (*stubCatalog) MarkReady(_ stdctx.Context) error             { return nil }
func (*stubCatalog) Ready() bool                                  { return true }
