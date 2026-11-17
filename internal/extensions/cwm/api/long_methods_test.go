// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"errors"
	"testing"
	"time"

	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// ,: the methods that move or rewrite a whole
// repository declare a request timeout of their own - the bus default (about
// 2 s) cut a group join of a 700-file dossier while the move went on, and a
// whole project's conversion from the GUI dialog.
func TestRepositoryWideMethodsDeclareATimeout(t *testing.T) {
	for _, m := range []string{"Migrate", "MigrateCommit"} {
		def, ok := CWMServiceDef.Methods[m]
		if !ok {
			t.Fatalf("%s is not declared", m)
		}
		if def.TimeoutS < 300 {
			t.Errorf("%s declares a timeout of %d s; want at least 300", m, def.TimeoutS)
		}
	}
}

// : the timeout a conversion method declares is the one a caller
// applies - read from the provider's manifest by the wire path the call
// goes out on (mbapi.MethodTimeout, which the GUI's call budget and the bus
// callers use), never the transport's default.
func TestConversionTimeoutReachesTheCall(t *testing.T) {
	st := mbapi.SlotType{Type: CWMServiceDef.Name}
	for name, def := range CWMServiceDef.Methods {
		st.Methods = append(st.Methods, mbapi.SlotMethod{Name: name, TimeoutS: def.TimeoutS})
	}
	man := &mbapi.Manifest{Provider: "ktw-agents", SlotTypes: []mbapi.SlotType{st}}
	get := func(_ context.Context, p string) (*mbapi.Manifest, error) {
		if p == man.Provider {
			return man, nil
		}
		return nil, errors.New("unknown provider")
	}
	for path, m := range map[string]string{
		"ktw-agents:cwm/migrate":        "Migrate",
		"ktw-agents:cwm/migrate-commit": "MigrateCommit",
	} {
		want := time.Duration(CWMServiceDef.Methods[m].TimeoutS) * time.Second
		got := mbapi.MethodTimeout(context.Background(), get, path)
		if got < 300*time.Second || got != want {
			t.Errorf("%s: a caller applies %v; want the declared %v (at least 5 min)", path, got, want)
		}
	}
}
