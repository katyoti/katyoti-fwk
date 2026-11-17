// SPDX-License-Identifier: Apache-2.0

package local

import (
	"reflect"
	"strings"
	"testing"
	"testing/fstest"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	gcapi "github.com/katyoti/katyoti-fwk/internal/extensions/gocode/api"
	gccore "github.com/katyoti/katyoti-fwk/internal/extensions/gocode/internal/core"
	personasapi "github.com/katyoti/katyoti-fwk/internal/extensions/personas/api"
	personaslocal "github.com/katyoti/katyoti-fwk/internal/extensions/personas/local"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// TestModuleOrchestrateSpawnsThroughPersonaService proves the persona verbs
// live in the personas extension: ModuleOrchestrate reaches PersonaService,
// and the answer it records is the personas service's own (no catalog here,
// so the persona is not found — reported with the personas sentinel, not
// CWM's). CWM itself carries no persona verb any more.
//
// The module is a software module: it IS a module because it carries the `module`
// capability, so ModuleOrchestrate accepts it (a test on the type's name
// refused it).
func TestModuleOrchestrateSpawnsThroughPersonaService(t *testing.T) {
	ctx, cwm := cwmapp.New(t, personaslocal.Extension(), cwmapp.Pack(fstest.MapFS{
		"object-types/software-module.yaml": {Data: cwmapp.ShippedFile(t, "app/ktw-code/embed/cwm/object-types/software-module.yaml")},
		"capabilities/code.yaml":            {Data: cwmapp.ShippedFile(t, "app/ktw-code/embed/cwm/capabilities/code.yaml")},
	}))
	mod := cwmapi.NewContainerObject("m", "m", "software-module", "", "m", "")
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "m", Object: mod}); err != nil {
		t.Fatalf("create software-module: %v", err)
	}

	out, err := (&gccore.GocodeServiceImpl{}).ModuleOrchestrate(ctx, gcapi.ModuleOrchestrateInput{
		ID: "m", Personas: []string{"reviewer"},
	})
	if err != nil {
		t.Fatalf("ModuleOrchestrate: %v", err)
	}
	if len(out.Spawns) != 1 {
		t.Fatalf("spawns = %+v, want one", out.Spawns)
	}
	rec := out.Spawns[0]
	if rec.Status != "error" || !strings.Contains(rec.Reason, personasapi.ErrNotFound.Error()) {
		t.Fatalf("spawn = %+v, want the PersonaService's not-found answer", rec)
	}

	cwmType := reflect.TypeOf((*cwmapi.CWMService)(nil)).Elem()
	for i := 0; i < cwmType.NumMethod(); i++ {
		if name := cwmType.Method(i).Name; strings.HasPrefix(name, "Persona") {
			t.Errorf("CWMService still carries %s — persona verbs belong to PersonaService", name)
		}
	}
	for key := range cwmapi.CWMServiceDef.Methods {
		if strings.HasPrefix(key, "persona.") {
			t.Errorf("CWMServiceDef still publishes %s", key)
		}
	}
}
