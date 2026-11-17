// SPDX-License-Identifier: Apache-2.0

package api

import (
	"reflect"
	"testing"

	corereflect "github.com/katyoti/katyoti-fwk/internal/core/reflect"
)

// TestSetupService_KTSPSurface guards which SetupService methods are KTSP-ready.
// DiscoverMethods exposes ONLY methods shaped (ctx, InputStruct)(Output, error) or
// (ctx)(Output, error) — the rest (bare-string params / error-only) are silently
// skipped until normalized. This test asserts the known-ready set stays exposed and
// logs the remaining normalization worklist (marketplace-service-ktsp Step B).
func TestSetupService_KTSPSurface(t *testing.T) {
	got := map[string]bool{}
	for _, m := range corereflect.DiscoverMethods(reflect.TypeOf((*SetupService)(nil)).Elem()) {
		got[m.Name] = true
	}
	// The KTSP-normalized methods MUST stay KTSP-exposable (grows as the
	// normalization sweep progresses — marketplace-service-ktsp).
	for _, name := range []string{
		"Doctor", "Install", "ListInstalled", "ListMarketplaces", "Outdated",
		// subscription group (normalized 2026-09-04):
		"AddMarketplace", "RemoveMarketplace", "SetMarketplaceEnabled", "RefreshCatalog",
		// catalog group (normalized 2026-09-04):
		"SearchItems", "GetItem", "ItemVersions",
		// install/update group (normalized 2026-09-04):
		"Uninstall", "Update", "SetVersion",
		// configure (normalized 2026-09-04):
		"ConfigureItem",
		// enable-orchestration (normalized 2026-09-05):
		"WireIntoUniverse", "Unwire", "InstallSolution", "RepinSolution",
		// the prereqs engine, for the GUI's Ready to start screen:
		"PlanPrereqs", "InstallPrereqs",
		// and its keys: the key sets with one password, then the one form:
		"KeySets", "ImportKeySets",
	} {
		if !got[name] {
			t.Errorf("SetupService.%s regressed off the KTSP surface (no longer (ctx,In)(Out,error))", name)
		}
	}
	// Infra MUTATION must stay OFF this surface. SetupService is served by a hub,
	// and these three replace katyoti.exe / ktw-hub.exe / ktw-gui.exe — so a hub
	// answering them would be replacing its own binary, and the GUI's, while both
	// run. The files survive (rename-aside) but the processes keep executing the
	// old code, making any success reported over the wire true about the disk and
	// false about the caller.
	//
	// They live on InfraService, which is registered in-process only. Putting one
	// back here would make that failure reachable again from the GUI.
	for _, name := range []string{"InstallInfra", "SelfUpdate", "SetInfraVersion"} {
		if got[name] {
			t.Errorf("SetupService.%s is KTSP-exposed — infra mutation must stay on the local-only InfraService", name)
		}
	}
	// The READS are fine over the wire, and the GUI needs them.
	for _, name := range []string{"InfraVersions", "InfraOutdated"} {
		if !got[name] {
			t.Errorf("SetupService.%s is missing — the GUI reads the control plane over KTSP", name)
		}
	}

	// Worklist: methods still needing Input/Output normalization to expose.
	iface := reflect.TypeOf((*SetupService)(nil)).Elem()
	for i := 0; i < iface.NumMethod(); i++ {
		if n := iface.Method(i).Name; !got[n] {
			t.Logf("normalize-to-expose: %s", n)
		}
	}
}
