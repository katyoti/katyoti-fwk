// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/render"
	panelapi "github.com/katyoti/katyoti-fwk/internal/extensions/panelserver/api"
)

// A page tree contributed as a page template is answered by its URI
// : its tree and its hash the first time, the hash alone to a
// host that holds it at that hash - a second answer naming the template
// fetches no tree - and the tree again once it changed. What names no
// template, a template not contributed and one off the page grammar are
// refused.
func TestTemplateAnswersItsTreeOnceAHostHoldsIt(t *testing.T) {
	bodies := map[string]string{
		"ktw-test:/page-template/steps": `{"label":"Étapes","tree":{"kind":"vbox","id":"steps","children":[{"kind":"text","id":"title","content":"Étapes"}]}}`,
		"ktw-test:/page-template/bad":   `{"tree":{"kind":"no-such-kind"}}`,
	}
	reads := 0
	s, err := New(Options{Dir: t.TempDir(), TemplateBody: func(_ context.Context, uri string) ([]byte, error) {
		reads++
		b, ok := bodies[uri]
		if !ok {
			return nil, errors.New("not found")
		}
		return []byte(b), nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	const uri = "ktw-test:/page-template/steps"

	first, err := s.Template(ctx, panelapi.TemplateInput{URI: uri})
	if err != nil {
		t.Fatal(err)
	}
	if first.Tree == nil || first.Tree["id"] != "steps" {
		t.Fatalf("the first answer carries no tree: %+v", first)
	}
	if want, _ := render.SpecHash(first.Tree); first.Hash != want || !strings.HasPrefix(first.Hash, "sha256:") {
		t.Errorf("the hash %q is not render.SpecHash of the tree (%q)", first.Hash, want)
	}
	// What an answer names travels as {uri, hash}: the host keeps that pair.
	named, _ := json.Marshal(render.SpecOf(uri, first.Hash))
	if string(named) != `{"uri":"`+uri+`","hash":"`+first.Hash+`"}` {
		t.Errorf("the template named in an answer: %s", named)
	}

	// A second answer naming it: the host holds it, no tree crosses.
	again, err := s.Template(ctx, panelapi.TemplateInput{URI: uri, Hash: first.Hash})
	if err != nil || again.Tree != nil || again.Hash != first.Hash {
		t.Errorf("a host holding the template got %+v (%v), want the hash alone", again, err)
	}

	// The template changed: a stale hash gets the new tree.
	bodies[uri] = strings.Replace(bodies[uri], `"content":"Étapes"`, `"content":"Les étapes"`, 1)
	changed, err := s.Template(ctx, panelapi.TemplateInput{URI: uri, Hash: first.Hash})
	if err != nil || changed.Tree == nil || changed.Hash == first.Hash {
		t.Errorf("a changed template with a stale hash: %+v (%v), want its new tree and hash", changed, err)
	}

	for name, in := range map[string]string{
		"no template URI":      "ktw-test:/panel-type/steps",
		"not contributed":      "ktw-test:/page-template/none",
		"off the page grammar": "ktw-test:/page-template/bad",
	} {
		if _, err := s.Template(ctx, panelapi.TemplateInput{URI: in}); err == nil {
			t.Errorf("%s (%s) was answered", name, in)
		}
	}
	if reads != 5 {
		t.Errorf("the catalog was read %d times, want once per call that names a template", reads)
	}
}
