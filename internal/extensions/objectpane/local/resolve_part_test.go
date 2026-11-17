// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	coreschemaapi "github.com/katyoti/katyoti-fwk/internal/core/schema/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/api"
	schemasvc "github.com/katyoti/katyoti-fwk/internal/extensions/schema/api"
)

// ONE SPECIFICATION PART BY ITS REFERENCE:
// a host whose cache misses asks the part a component draws; it comes back
// with render.SpecHash of it, nothing but the hash when the host holds the
// current one, a new hash once the data type changes, and a part whose
// fields compute their choices is never cacheable.

// partSchemas is a schema authority whose data types can change.
type partSchemas struct {
	schemasvc.SchemaService
	mu    sync.Mutex
	books map[string]string // URI -> the data type's JSON
}

func (s *partSchemas) set(uri, body string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.books[uri] = body
}

func (s *partSchemas) GetDataTypeWithExtensions(_ context.Context, in schemasvc.GetDataTypeWithExtensionsInput) (schemasvc.GetDataTypeOutput, error) {
	s.mu.Lock()
	body, ok := s.books[in.URI]
	s.mu.Unlock()
	if !ok {
		return schemasvc.GetDataTypeOutput{}, schemasvc.ErrDataTypeNotFound
	}
	var sch coreschemaapi.Schema
	if err := json.Unmarshal([]byte(body), &sch); err != nil {
		return schemasvc.GetDataTypeOutput{}, err
	}
	return schemasvc.GetDataTypeOutput{Schema: &sch}, nil
}

const (
	bookURI  = "ktw-demo:/data-type/book"
	bookV1   = `{"name":"book","root":"Book","types":{"Book":{"type":"object","fields":{"title":{"type":"string"}}}}}`
	bookV2   = `{"name":"book","root":"Book","types":{"Book":{"type":"object","fields":{"title":{"type":"string"},"isbn":{"type":"string"}}}}}`
	pickURI  = "ktw-demo:/data-type/pick"
	pickBody = `{"name":"pick","root":"In","types":{"In":{"type":"object","fields":{"thing":{"type":"string","content":"text","contentParams":{"options_from":{"source":"objects","types":["thing"],"label":"name"}}}}}}}`
)

func partWorld(t *testing.T) (context.Context, *partSchemas, api.ObjectPaneService) {
	t.Helper()
	sch := &partSchemas{books: map[string]string{bookURI: bookV1, pickURI: pickBody}}
	ctx, _, svc := paneWorld(t, map[string]string{"object-types/thing.yaml": "name: thing\ntree: node\n"},
		application.DeclareSimpleService[schemasvc.SchemaService](func(context.Context) (schemasvc.SchemaService, error) { return sch, nil }),
	)
	return ctx, sch, svc
}

func TestResolvePartAnswersThePartWithItsHash(t *testing.T) {
	ctx, sch, svc := partWorld(t)
	first, err := svc.ResolvePart(ctx, api.ResolvePartInput{Kind: "view", Schema: bookURI})
	if err != nil {
		t.Fatal(err)
	}
	want, _ := render.SpecHash(first.Part)
	if first.Part == nil || first.Hash != want || !first.Cacheable {
		t.Fatalf("the part comes back whole, cacheable, with render.SpecHash of it: %+v (want hash %s)", first, want)
	}
	if first.Part["root"] != "Book" {
		t.Errorf("the part is the data type's AST: %v", first.Part)
	}

	held, err := svc.ResolvePart(ctx, api.ResolvePartInput{Kind: "view", Schema: bookURI, Hash: first.Hash})
	if err != nil {
		t.Fatal(err)
	}
	if held.Part != nil || held.Hash != first.Hash {
		t.Errorf("a host holding the current hash gets the hash alone: %+v", held)
	}

	sch.set(bookURI, bookV2)
	changed, err := svc.ResolvePart(ctx, api.ResolvePartInput{Kind: "view", Schema: bookURI, Hash: first.Hash})
	if err != nil {
		t.Fatal(err)
	}
	if changed.Part == nil || changed.Hash == first.Hash {
		t.Errorf("a changed data type answers its new part and a new hash, never a refusal: %+v", changed)
	}
}

func TestResolvePartOfComputedChoicesIsNotCacheable(t *testing.T) {
	ctx, _, svc := partWorld(t)
	out, err := svc.ResolvePart(ctx, api.ResolvePartInput{Kind: "form", Schema: pickURI})
	if err != nil {
		t.Fatal(err)
	}
	if out.Cacheable || out.Part == nil {
		t.Errorf("a part whose fields compute their choices comes back whole and not cacheable: %+v", out)
	}
	again, err := svc.ResolvePart(ctx, api.ResolvePartInput{Kind: "form", Schema: pickURI, Hash: out.Hash})
	if err != nil {
		t.Fatal(err)
	}
	if again.Part == nil {
		t.Error("a part that is not cacheable is answered whole even when its hash is held")
	}
}

func TestResolvePartRefusesWhatIsNoPart(t *testing.T) {
	ctx, _, svc := partWorld(t)
	if _, err := svc.ResolvePart(ctx, api.ResolvePartInput{Kind: "markdown", Schema: bookURI}); err == nil {
		t.Error("a kind that draws no schema is refused")
	}
	if _, err := svc.ResolvePart(ctx, api.ResolvePartInput{Kind: "view", Schema: map[string]any{"root": "X"}}); err == nil {
		t.Error("an inline schema is no reference: refused")
	}
	if _, err := svc.ResolvePart(ctx, api.ResolvePartInput{Kind: "view", Schema: "ktw-demo:/data-type/none"}); err == nil {
		t.Error("an unknown data type names itself as an error")
	}
}
