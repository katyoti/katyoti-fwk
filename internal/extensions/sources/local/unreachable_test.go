// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
	sourcescontrib "github.com/katyoti/katyoti-fwk/internal/extensions/sources/contrib"
	sourceslocal "github.com/katyoti/katyoti-fwk/internal/extensions/sources/local"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// unreachableWhy is what the out-of-reach connector says, in every answer:
// each surface must carry it verbatim.
const unreachableWhy = "Hôte injoignable : monitoring.example hors du réseau ACME"

// Two connectors side by side: far cannot be reached (its test answers not
// ok with the why, its sync and browse fail with it), near reads.
const (
	urFar = `name: far
label: Supervision
description: Entities of a monitoring tenant out of reach.
sync: ktw-ur:far/source-sync
browse: ktw-ur:far/browse
test: ktw-ur:far/test
propose: ktw-ur:far/propose
`
	urNear = `name: near
label: Cartographie
description: The applications of the map.
sync: ktw-ur:near/source-sync
browse: ktw-ur:near/browse
propose: ktw-ur:near/propose
`
)

// urActions serves the two fake connectors.
type urActions struct{}

func (urActions) Describe(context.Context, actionsapi.DescribeInput) (actionsapi.DescribeOutput, error) {
	return actionsapi.DescribeOutput{}, nil
}

func (urActions) Run(ctx context.Context, in actionsapi.RunInput) (actionsapi.RunOutput, error) {
	var value any
	switch in.URI {
	case "ktw-ur:far/test":
		value = api.ConnectorTestOutput{OK: false, Message: unreachableWhy}
	case "ktw-ur:far/source-sync", "ktw-ur:far/browse", "ktw-ur:far/propose":
		return actionsapi.RunOutput{}, errors.New(unreachableWhy)
	case "ktw-ur:near/source-sync":
		var sin api.ConnectorSyncInput
		_ = json.Unmarshal(in.Input, &sin)
		if err := os.WriteFile(filepath.Join(sin.Dest, "erp.md"), []byte("# ERP\n\nThe ERP.\n"), 0o644); err != nil {
			return actionsapi.RunOutput{}, err
		}
		value = api.ConnectorSyncOutput{Items: []api.ConnectorItem{{Path: "erp.md", URI: "maps://erp", Title: "ERP", Key: "erp"}}}
	case "ktw-ur:near/browse":
		value = api.ConnectorBrowseOutput{Items: []api.BrowseItem{{Key: "erp", Title: "ERP"}}}
	case "ktw-ur:near/propose":
		value = api.ConnectorProposeOutput{Items: []api.ProposedItem{{PickItem: api.PickItem{Key: "erp", Title: "ERP"}, Why: "Trouvée par « achats »", Score: 1}}}
	case "ktw-agents:sources/source-check":
		value = cwmapi.HookAccept{Accept: true}
	default:
		return actionsapi.RunOutput{}, fmt.Errorf("no method %s", in.URI)
	}
	raw, _ := json.Marshal(value)
	return actionsapi.RunOutput{URI: in.URI, Value: raw}, nil
}

// (for): a source that cannot be reached
// says why - verbatim, the connector's own words - on every sources surface
// (its board card, the search, the browse of its picker), while a source
// beside it syncs, is browsed and is found as usual.
func TestAnUnreachableSourceSaysWhyWhileTheOthersGoOn(t *testing.T) {
	ms := loader.NewEmbedFSLoader(fstest.MapFS{
		"connectors/far.yaml":  {Data: []byte(urFar)},
		"connectors/near.yaml": {Data: []byte(urNear)},
	}, ".")
	ctx, cwm := cwmapp.NewAt(t, t.TempDir(),
		sourcescontrib.Pack("ktw-agents"),
		sourcescontrib.DeclareKinds("ktw-agents"),
		contributeConnectors("ktw-ur", ms, "connectors"),
		cwmapp.Pack(fstest.MapFS{
			"object-types/module.yaml":    {Data: []byte("name: module\ntree: node\n")},
			"capabilities/lifecycle.yaml": {Data: []byte("name: lifecycle\n")},
		}),
		sourceslocal.Extension(),
		application.DeclareSimpleService[actionsapi.ActionService](func(context.Context) (actionsapi.ActionService, error) {
			return urActions{}, nil
		}),
	)
	cwmapp.Module(t, ctx, cwm, "m")
	if _, err := cwm.ObjectAddCapability(ctx, cwmapi.ObjectAddCapabilityInput{ID: "m", Capability: api.KnowledgeCapability}); err != nil {
		t.Fatal(err)
	}
	svc, err := application.UseService[api.SourcesService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]string{}
	for _, name := range []string{"far", "near"} {
		used, err := svc.Use(ctx, api.UseInput{ID: "m", Connector: name})
		if err != nil || used.Source == "" {
			t.Fatalf("use %s: %v %+v", name, err, used)
		}
		ids[name] = used.Source
	}

	// Sync: far fails saying why, near reads its item.
	far, err := svc.SourceSync(ctx, api.SourceSyncInput{ID: ids["far"]})
	if err != nil {
		t.Fatalf("a failed sync is an outcome, not an error: %v", err)
	}
	if far.OK || far.State != api.SourceStateFailed || !strings.Contains(far.Message, unreachableWhy) {
		t.Errorf("far's sync fails saying why: %+v", far)
	}
	near, err := svc.SourceSync(ctx, api.SourceSyncInput{ID: ids["near"]})
	if err != nil || !near.OK || near.Items != 1 {
		t.Fatalf("near syncs beside the unreachable one: %+v %v", near, err)
	}

	// The board's cards: far « injoignable », red, the why as its note;
	// near « à jour ».
	of, err := svc.SourcesOf(ctx, api.SourcesOfInput{ID: "m"})
	if err != nil {
		t.Fatal(err)
	}
	cards := map[string]api.ConnectorUse{}
	for _, c := range of.Connectors {
		cards[c.Name] = c
	}
	if c := cards["far"]; c.State == nil || c.State.Text != "injoignable" || c.State.Color != "red" || c.Note != unreachableWhy {
		t.Errorf("far's card reads « injoignable » with the connector's why verbatim: %+v state %+v", c, c.State)
	}
	if c := cards["near"]; c.State == nil || c.State.Text != "à jour" || c.Note != "" {
		t.Errorf("near's card is up to date: %+v state %+v", c, c.State)
	}

	// The search: far unreachable with the why, never asked to propose;
	// near found.
	out, err := svc.SearchSources(ctx, api.SearchSourcesInput{Terms: []string{"achats"}})
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]api.SourceSearch{}
	for _, s := range out.Sources {
		found[s.Connector] = s
	}
	if s := found["far"]; s.State != api.SearchUnreachable || s.Message != unreachableWhy || s.Found != 0 {
		t.Errorf("far's search is unreachable saying why: %+v", s)
	}
	if s := found["near"]; s.State != api.SearchFound || s.Found != 1 {
		t.Errorf("near is found beside the unreachable one: %+v", s)
	}

	// The browse of far's picker: the error carries the why, readable;
	// near's picker lists its item.
	if _, err := svc.SourceBrowse(ctx, api.SourceBrowseInput{ID: ids["far"]}); err == nil || err.Error() != unreachableWhy {
		t.Errorf("far's browse fails with the connector's why verbatim: %v", err)
	}
	if got, err := svc.SourceBrowse(ctx, api.SourceBrowseInput{ID: ids["near"]}); err != nil || len(got.Items) != 1 || got.Items[0].Key != "erp" {
		t.Errorf("near's browse lists its item: %+v %v", got, err)
	}
}
