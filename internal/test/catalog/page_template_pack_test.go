// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"context"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/contribution"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	manifestcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/contrib"
)

// A pack adds a page template with a file and no code: its
// page-templates/<name>.yaml is read as the system kind declares - a
// <pack>:/page-template/<name> entry, its data the template - and its tree is
// a valid page that an answer names by that URI and its hash.
func TestAPackAddsAPageTemplateWithNoCode(t *testing.T) {
	kinds, err := contribution.SystemKinds()
	if err != nil {
		t.Fatal(err)
	}
	body := "label: Étapes\ntree:\n  kind: vbox\n  id: steps\n  children:\n    - {kind: text, id: title, content: Étapes}\n"
	pack := loader.NewMapLoader(map[string]string{"page-templates/steps.yaml": body})
	var uris []string
	for _, p := range manifestcontrib.PackProviders("ktw-test", pack, ".", kinds) {
		if p.Type() != render.PageTemplateKind {
			continue
		}
		got, err := p.Enumerate(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range got {
			uris = append(uris, c.ResolvedURI())
		}
	}
	if len(uris) != 1 || uris[0] != "ktw-test:/page-template/steps" || !render.IsPageTemplateURI(uris[0]) {
		t.Fatalf("the pack's page-templates/ folder gives %v, want the one entry ktw-test:/page-template/steps", uris)
	}
	var tpl render.PageTemplate
	if err := config.DecodeBytes([]byte(body), marshal.YAML, &tpl); err != nil {
		t.Fatal(err)
	}
	if err := tpl.Validate(); err != nil {
		t.Fatalf("the pack's template is no valid page: %v", err)
	}
	spec, err := tpl.Spec(uris[0])
	if err != nil || !spec.Named() || spec.Hash == "" {
		t.Fatalf("the template named in an answer: %+v %v", spec, err)
	}
}
