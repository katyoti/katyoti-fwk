// SPDX-License-Identifier: Apache-2.0

package contribapi

import (
	"reflect"
	"testing"
)

// The envelope carries NO read preference. Which layer a consumer wants is
// the consumer's question: the body lane asks for `data`, the content lane
// asks for the layer it was given, and the kind names the default. A method
// on this type that picks a layer for the caller is exactly what put YAML in
// front of a JSON decoder (`DataOrSource`) and markdown behind an
// application/json label (`Content`). This guard refuses their return.
//
// Layer access is by name — Data(), Source(), Layers[name] — and nothing else.
func TestEnvelopeCarriesNoReadPreference(t *testing.T) {
	forbidden := map[string]string{
		"Content":      "source-first preference; use Open(uri, layer) with the kind's default",
		"DataOrSource": "data-first preference; the body lane is Data() and nothing else",
		"HasBulkLayer": "band questions are per layer: LayerRef.IsBulk()",
		"Body":         "the pre-layer body field; the data layer is Data()",
	}
	// The pointer type's method set includes the value's AND the
	// pointer-receiver methods (DropInline, SetData, SetSource, SetLayer), so
	// a preference cannot come back on a pointer receiver unseen.
	for _, mt := range []reflect.Type{reflect.TypeOf(Contribution{}), reflect.TypeOf(&Contribution{})} {
		for i := 0; i < mt.NumMethod(); i++ {
			name := mt.Method(i).Name
			if why, bad := forbidden[name]; bad {
				t.Errorf("(%s).%s is back — %s", mt, name, why)
			}
		}
	}
	if n := reflect.TypeOf(&Contribution{}).NumMethod(); n <= reflect.TypeOf(Contribution{}).NumMethod() {
		t.Fatalf("the pointer method set (%d) is not wider than the value's: the guard would not see pointer receivers", n)
	}
	rt := reflect.TypeOf(Contribution{})
	for i := 0; i < rt.NumField(); i++ {
		name := rt.Field(i).Name
		if why, bad := forbidden[name]; bad {
			t.Errorf("Contribution.%s field is back — %s", name, why)
		}
	}
}
