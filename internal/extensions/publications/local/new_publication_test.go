// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/publications/api"
)

// TestANewPublicationIsCreatedUnderWhatItPublishes (mock-up
// v6 view 29): the publication type's create page offers one choice per
// contributed connector, names the publication after the one chosen and
// creates only once both are set; « Créer » makes a publication under its
// holder, publishing the holder through the connector, and opens it; an
// unknown connector is refused, nothing created.
func TestANewPublicationIsCreatedUnderWhatItPublishes(t *testing.T) {
	ctx, svc, _ := publishersWorld(t)
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	holder := cwmapi.NewContainerObject("paie", "paie", "module", "", "Paie", "")
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "paie", Object: holder}); err != nil {
		t.Fatal(err)
	}

	page, err := svc.NewPublicationPage(ctx, api.NewPublicationPageInput{Parent: "paie"})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(page.DSLTree)
	for _, want := range []string{`"kind":"choice-cards"`, `"value":"wiki"`, `"label":"Wiki"`, `"value":"folder"`, `"sets":{"name":"Wiki"}`,
		`"uri":"` + api.PublicationNewURI + `"`, `"parent":"paie"`, `"requires":["connector","name"]`, "onglet Destination"} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("the create page lacks %s: %s", want, raw)
		}
	}

	out, err := svc.PublicationNew(ctx, api.PublicationNewInput{Parent: "paie", Connector: "wiki", Name: "Wiki — espace ARCHI"})
	if err != nil || !out.OK || out.Open == nil || out.Open.Object != out.Publication {
		t.Fatalf("created and opened: %+v %v", out, err)
	}
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{ID: out.Publication})
	if err != nil {
		t.Fatal(err)
	}
	var f api.PublicationFields
	b, _ := json.Marshal(got.Object.Capabilities[api.CapPublication])
	_ = json.Unmarshal(b, &f)
	if got.Object.Type != api.PublicationType || got.Object.Parent != "paie" || got.Object.Name != "Wiki — espace ARCHI" || f.Object != "paie" || f.Connector != "wiki" {
		t.Fatalf("a publication of its holder through the connector: %+v %+v", got.Object, f)
	}
	second, err := svc.PublicationNew(ctx, api.PublicationNewInput{Parent: "paie", Connector: "wiki"})
	if err != nil || !second.OK || second.Publication == out.Publication {
		t.Fatalf("a second publication of the same connector has its own id: %+v %v", second, err)
	}
	if bad, err := svc.PublicationNew(ctx, api.PublicationNewInput{Parent: "paie", Connector: "nowhere"}); err != nil || bad.OK {
		t.Fatalf("an unknown connector created a publication: %+v %v", bad, err)
	}
	if _, err := svc.PublicationNew(context.Background(), api.PublicationNewInput{Connector: "wiki"}); err == nil {
		t.Fatal("a publication with no holder was created")
	}
}
