// SPDX-License-Identifier: Apache-2.0

package local

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	corereflect "github.com/katyoti/katyoti-fwk/internal/core/reflect"
	panelapi "github.com/katyoti/katyoti-fwk/internal/extensions/panelserver/api"
	panelcore "github.com/katyoti/katyoti-fwk/internal/extensions/panelserver/internal/core"
)

// The panel server's KTSP slot publishes PanelService's methods and no other
// : the container publishes every method of the value it is
// given, so the server's own - Prune, Take, Input, Follow - stay off the bus.
func TestThePanelServerPublishesOnlyItsReads(t *testing.T) {
	names := func(typ reflect.Type) string {
		var out []string
		for _, m := range corereflect.DiscoverMethods(typ) {
			out = append(out, m.Name)
		}
		sort.Strings(out)
		return strings.Join(out, ",")
	}
	want := names(reflect.TypeOf((*panelapi.PanelService)(nil)).Elem())
	if got := names(reflect.TypeOf(published{})); got != want {
		t.Fatalf("the slot publishes %s, want PanelService's %s alone", got, want)
	}
	if got := names(reflect.TypeOf(&panelcore.Server{})); !strings.Contains(got, "Prune") {
		t.Fatalf("the server's own methods are %s: the guard no longer sees Prune", got)
	}
	srv, err := panelcore.New(panelcore.Options{Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	got, err := serverOf(published{PanelService: srv, srv: srv})
	if err != nil || got != srv {
		t.Fatalf("serverOf = %v, %v; want the server behind the published service", got, err)
	}
	if _, err := serverOf(srv); err == nil {
		t.Fatal("a bare server was taken for the published service")
	}
}
