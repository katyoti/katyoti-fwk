// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"context"
	"sort"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/contribution"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	"github.com/katyoti/katyoti-fwk/internal/core/panel"
	manifestcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/contrib"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/terminal/chatpanel"
)

// KaTyoti ships its generic panel types as panel-type contributions of
// ktw-agents: one entry per type, each body a valid panel
// type equal to Go's declaration - among them the types a method run and a
// run's conversation open.
func TestKatyotiShipsTheGenericPanelTypes(t *testing.T) {
	kinds, err := contribution.SystemKinds()
	if err != nil {
		t.Fatal(err)
	}
	files, err := Files()
	if err != nil {
		t.Fatal(err)
	}
	var uris []string
	for _, p := range manifestcontrib.PackProviders("ktw-agents", loader.NewMapLoader(files), Root, kinds) {
		if p.Type() != "panel-type" {
			t.Errorf("the pack publishes a %s", p.Type())
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
	sort.Strings(uris)
	want := panel.GenericTypeURIs()
	if len(uris) != len(want) {
		t.Fatalf("published %v, want %v", uris, want)
	}
	for i := range want {
		if uris[i] != want[i] {
			t.Fatalf("published %v, want %v", uris, want)
		}
	}
	for path, body := range files {
		var decl panel.TypeDecl
		if err := config.DecodeBytes([]byte(body), marshal.YAML, &decl); err != nil {
			t.Fatalf("%s does not read: %v", path, err)
		}
		if err := decl.Validate(); err != nil {
			t.Errorf("%s: %v", path, err)
		}
	}
	for _, uri := range []string{taskapi.ProgressPanelType, chatpanel.Type} {
		if _, ok := panel.GenericTypes()[uri]; !ok {
			t.Errorf("%s is opened by KaTyoti and is no generic type", uri)
		}
	}
}
