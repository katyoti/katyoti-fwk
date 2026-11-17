// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
	sourcescontrib "github.com/katyoti/katyoti-fwk/internal/extensions/sources/contrib"
	sourceslocal "github.com/katyoti/katyoti-fwk/internal/extensions/sources/local"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// On a universe with three sidecars (the made-up ktw-sample, ktw-example and
// ktw-demo), the sources pack lists their connectors beside its own - each
// from the sidecar's embedded entry, contributed through the
// source-connector kind, each naming its sync and test methods
func TestTheSidecarsConnectorsAreListed(t *testing.T) {
	ctx, _ := cwmapp.New(t,
		sourcescontrib.Pack("ktw-agents"),
		sourcescontrib.DeclareKinds("ktw-agents"),
		sourcescontrib.OwnConnectors("ktw-agents"),
		exampleConnectors("ktw-sample"),
		exampleConnectors("ktw-example"),
		exampleConnectors("ktw-demo"),
		cwmapp.Pack(fstest.MapFS{"object-types/module.yaml": {Data: []byte("name: module\ntree: node\n")}}),
		sourceslocal.Extension(),
	)
	svc, err := application.UseService[api.SourcesService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	out, err := svc.Connectors(ctx, api.ConnectorsInput{})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]api.ConnectorDecl{}
	for _, c := range out.Connectors {
		got[c.Name] = c
	}
	want := map[string]string{
		api.FolderConnector:   "ktw-agents:sources/folder-sync",
		"sample-pages":        "ktw-sample:sample/source-sync",
		"example-application": "ktw-example:example/source-sync",
		"demo-entities":       "ktw-demo:demo/source-sync",
	}
	for name, sync := range want {
		c, ok := got[name]
		if !ok || c.Sync != sync {
			t.Errorf("connector %s: %+v (listed %v); want its sync %s", name, c, ok, sync)
			continue
		}
		// A connector picking by search across its place needs no location
		// form (sample-pages:).
		if name != api.FolderConnector && (c.Test == "" || c.Label == "" || (c.Location == "" && c.Search == "")) {
			t.Errorf("connector %s lacks its test method, label or location form: %+v", name, c)
		}
	}
}
