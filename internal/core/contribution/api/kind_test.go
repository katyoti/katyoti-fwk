// SPDX-License-Identifier: Apache-2.0

package contribapi

import "testing"

// DefaultLayer follows the declaration first and the data/source convention
// second, so a half-written declaration still opens something.
func TestKindDeclaration_DefaultLayer(t *testing.T) {
	cases := []struct {
		name string
		decl KindDeclaration
		want string
	}{
		{"none declared", KindDeclaration{}, ""},
		{"explicit default", KindDeclaration{Layers: map[string]LayerDecl{"data": {}, "source": {Default: true}}}, "source"},
		{"data explicit default", KindDeclaration{Layers: map[string]LayerDecl{"data": {Default: true}, "source": {}}}, "data"},
		{"convention: source before data", KindDeclaration{Layers: map[string]LayerDecl{"data": {}, "source": {}}}, "source"},
		{"convention: data alone", KindDeclaration{Layers: map[string]LayerDecl{"data": {}}}, "data"},
	}
	for _, c := range cases {
		if got := c.decl.DefaultLayer(); got != c.want {
			t.Errorf("%s: DefaultLayer() = %q, want %q", c.name, got, c.want)
		}
	}
}
