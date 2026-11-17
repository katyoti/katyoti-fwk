// SPDX-License-Identifier: Apache-2.0

package api

import (
	"reflect"
	"sort"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
)

// interface == Def: the Go interface a consumer codes against and the
// ServiceDef the wire advertises name the same methods. Before e14,
// CatalogService declared 6 methods while CatalogServiceDef declared 7
// (OpenLayer was published only because KTSP reflects the concrete type).
func TestCatalogServiceInterfaceMatchesDef(t *testing.T) {
	assertInterfaceMatchesDef(t, reflect.TypeOf((*CatalogService)(nil)).Elem(), CatalogServiceDef)
}

func TestResourceServiceInterfaceMatchesDef(t *testing.T) {
	assertInterfaceMatchesDef(t, reflect.TypeOf((*ResourceService)(nil)).Elem(), ResourceServiceDef)
}

func assertInterfaceMatchesDef(t *testing.T, iface reflect.Type, def meta.ServiceDef) {
	t.Helper()
	var goNames, defNames []string
	for i := 0; i < iface.NumMethod(); i++ {
		goNames = append(goNames, iface.Method(i).Name)
	}
	for name := range def.Methods {
		defNames = append(defNames, name)
	}
	sort.Strings(goNames)
	sort.Strings(defNames)
	if !reflect.DeepEqual(goNames, defNames) {
		t.Fatalf("%s: Go interface methods %v != ServiceDef methods %v", iface.Name(), goNames, defNames)
	}
}
