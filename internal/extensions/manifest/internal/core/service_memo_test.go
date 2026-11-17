// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"io"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/em"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// service_memo_test.go — the self-reach verbs read ONE build of an
// immutable binary's manifest.
//
// Every self-reach verb (List, Get, GetManifest, Types, GetDataType) reads
// the built manifest. A build enumerates every provider and hashes the whole
// manifest's JSON; for a binary whose providers are all immutable the result
// cannot change, so building it per call is pure cost. A single mutable
// provider makes the whole build live again.

// countingProvider counts its enumerations: one per build, so the count IS
// the number of builds this provider took part in.
type countingProvider struct {
	name, typ    string
	mutable      bool
	entries      []contribapi.Contribution
	enumerations atomic.Int32
}

func (p *countingProvider) Type() string     { return p.typ }
func (p *countingProvider) Provider() string { return p.name }
func (p *countingProvider) Enumerate(context.Context) ([]contribapi.Contribution, error) {
	p.enumerations.Add(1)
	return p.entries, nil
}
func (p *countingProvider) Identity() contribapi.Identity {
	if p.mutable {
		return contribapi.Identity{Mutable: true}
	}
	return contribapi.Identity{Digest: contribapi.DigestOf(p.entries)}
}
func (p *countingProvider) Open(ctx context.Context, id, layer string) (io.ReadCloser, contribapi.LayerRef, error) {
	return contribapi.OpenEnumerated(ctx, p, id, layer)
}

// skillEntry carries a control-band data layer (past BandInline), so a
// bodiless List has something to strip and a with-body one something to keep.
func skillEntry(provider, id string) contribapi.Contribution {
	c := contribapi.Contribution{Provider: provider, Type: "skill", ID: id}
	c.SetData([]byte(`{"name":"` + id + `","pad":"` + strings.Repeat("x", int(contribapi.BandInline)+1) + `"}`))
	c.SetSource([]byte("---\nname: "+id+"\n---\n# "+id+"\n"), "text/markdown")
	return c
}

// providersContext runs a fresh Application whose only extension point is the
// contribution providers', filled with the given providers.
func providersContext(t *testing.T, providers ...contribapi.ContributionProvider) context.Context {
	t.Helper()
	app := application.New(application.Config(), func(init *application.ApplicationInit) {
		init.AddRegisterPoints("test", "providers", func(app *application.Application) error {
			return application.RegisterPoint(app, em.NewListPoint[contribapi.ContributionProvider]())
		})
		init.AddContribute("test", "providers", func(app *application.Application) error {
			for _, p := range providers {
				if err := application.Contribute(app, p); err != nil {
					return err
				}
			}
			return nil
		})
	})
	if err := app.Run(); err != nil {
		t.Fatalf("app.Run: %v", err)
	}
	return app.Context()
}

// everySelfVerb runs each manifest-reading verb once.
func everySelfVerb(t *testing.T, ctx context.Context, s *ResourceServiceImpl) {
	t.Helper()
	if _, err := s.List(ctx, mbapi.ContributionListInput{}); err != nil {
		t.Fatalf("List: %v", err)
	}
	if _, err := s.Get(ctx, mbapi.ContributionGetInput{URI: "ktw-test:/skill/a"}); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if _, err := s.GetManifest(ctx, mbapi.ContributionGetManifestInput{}); err != nil {
		t.Fatalf("GetManifest: %v", err)
	}
	if _, err := s.Types(ctx); err != nil {
		t.Fatalf("Types: %v", err)
	}
	if _, err := s.GetDataType(ctx, mbapi.ContributionGetDataTypeInput{Name: "x"}); err != nil {
		t.Fatalf("GetDataType: %v", err)
	}
}

func TestResourceService_ImmutableSetIsBuiltOnce(t *testing.T) {
	a := &countingProvider{name: "ktw-test", typ: "skill", entries: []contribapi.Contribution{skillEntry("ktw-test", "a")}}
	sys := &countingProvider{name: contribapi.SystemProvider, typ: "skill", entries: []contribapi.Contribution{skillEntry(contribapi.SystemProvider, "sys")}}
	ctx := providersContext(t, a, sys)
	s := NewResourceService("ktw-test", "dev")

	const rounds = 5
	for i := 0; i < rounds; i++ {
		everySelfVerb(t, ctx, s)
	}
	if n := a.enumerations.Load(); n != 1 {
		t.Errorf("%d rounds of every self-reach verb built the manifest %d times, want 1: an immutable set is built once", rounds, n)
	}

	// The memo is never handed out by reference: a caller that narrows its
	// manifest (GetManifest without system) or strips bodies (List without
	// bodies) changes its own copy, and the next caller sees the whole.
	no := false
	narrowed, err := s.GetManifest(ctx, mbapi.ContributionGetManifestInput{IncludeSystem: &no})
	if err != nil {
		t.Fatal(err)
	}
	if len(narrowed.Manifest.Contributions) != 1 {
		t.Fatalf("narrowed manifest has %d entries, want 1", len(narrowed.Manifest.Contributions))
	}
	whole, err := s.GetManifest(ctx, mbapi.ContributionGetManifestInput{})
	if err != nil {
		t.Fatal(err)
	}
	if len(whole.Manifest.Contributions) != 2 || whole.Manifest.Hash == narrowed.Manifest.Hash {
		t.Errorf("after a narrowed GetManifest the whole manifest has %d entries (hash %s vs narrowed %s): the memo was mutated through a caller's copy",
			len(whole.Manifest.Contributions), whole.Manifest.Hash, narrowed.Manifest.Hash)
	}
	if _, err := s.List(ctx, mbapi.ContributionListInput{WithBody: false}); err != nil {
		t.Fatal(err)
	}
	withBody, err := s.List(ctx, mbapi.ContributionListInput{WithBody: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range withBody.Contributions {
		if c.Data() == nil {
			t.Errorf("%s: a with-body List after a bodiless one carries no data bytes: DropInline reached the memo", c.URI)
		}
	}
	if n := a.enumerations.Load(); n != 1 {
		t.Errorf("built %d times in total, want 1", n)
	}
}

func TestResourceService_OneMutableProviderMakesEveryCallBuild(t *testing.T) {
	a := &countingProvider{name: "ktw-test", typ: "skill", entries: []contribapi.Contribution{skillEntry("ktw-test", "a")}}
	live := &countingProvider{name: "ktw-test", typ: "reference", mutable: true}
	ctx := providersContext(t, a, live)
	s := NewResourceService("ktw-test", "dev")

	const rounds = 3
	for i := 0; i < rounds; i++ {
		everySelfVerb(t, ctx, s)
	}
	// Five verbs per round, each a build: a mutable source is re-read on
	// every call, as before.
	if n := live.enumerations.Load(); n != rounds*5 {
		t.Errorf("a mutable provider was enumerated %d times over %d calls, want one per call", n, rounds*5)
	}
	if n := a.enumerations.Load(); n != rounds*5 {
		t.Errorf("the immutable sibling was enumerated %d times, want %d: a build is all providers or none", n, rounds*5)
	}
}

// A memo belongs to the provider set it was built from. The same service
// asked under an application with another set builds again.
func TestResourceService_MemoDoesNotOutliveItsProviderSet(t *testing.T) {
	a := &countingProvider{name: "ktw-test", typ: "skill", entries: []contribapi.Contribution{skillEntry("ktw-test", "a")}}
	b := &countingProvider{name: "ktw-test", typ: "skill", entries: []contribapi.Contribution{skillEntry("ktw-test", "b")}}
	one := providersContext(t, a)
	two := providersContext(t, a, b)
	s := NewResourceService("ktw-test", "dev")

	if _, err := s.Types(one); err != nil {
		t.Fatal(err)
	}
	out, err := s.List(two, mbapi.ContributionListInput{})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Contributions) != 2 {
		t.Errorf("under a two-provider application List returned %d entries, want 2: the one-provider memo was served", len(out.Contributions))
	}
	if n := b.enumerations.Load(); n != 1 {
		t.Errorf("the provider the second set adds was enumerated %d times, want 1", n)
	}
}
