// SPDX-License-Identifier: Apache-2.0

package census

import (
	"fmt"
	"mime"
	"sort"
	"strings"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
)

// assertLayerMediaTypesMatchDeclaration: a layer is the kind's named content
// location, so what an entry carries under a name must be what the kind
// declares under it (e14). Two findings:
//
//   - ambiguity — a layer name two ways (`source` and `Source` on one entry,
//     or declared twice by one kind modulo case), or not in the lower-case
//     form every lane looks it up by. A map key cannot repeat, so a duplicate
//     name only exists as a case variant; that is the one to refuse.
//   - media type — the kind declares a layer's media type and the entry's
//     layer carries a different one (parameters such as `charset` ignored).
//     A kind that declares no media type for a layer (reference `source`:
//     whatever the file is) is not checked.
func assertLayerMediaTypesMatchDeclaration(t *testing.T, name string, kinds contribapi.Kinds, cs []contribapi.Contribution) {
	t.Helper()
	bad := layerDeclarationMismatches(kinds, cs)
	if len(bad) == 0 {
		return
	}
	t.Errorf("census[%s]: %d layer(s) contradict their kind's declaration:\n%s", name, len(bad), strings.Join(bad, "\n"))
}

// layerDeclarationMismatches is the decision the assertion reports: one line
// per finding, sorted. Pure, so the guard is falsified without a binary.
func layerDeclarationMismatches(kinds contribapi.Kinds, cs []contribapi.Contribution) []string {
	var bad []string
	for _, kind := range kinds.Names() {
		d, _ := kinds.Get(kind)
		bad = append(bad, ambiguousLayerNames(fmt.Sprintf("kind %q declares", kind), keys(d.Layers))...)
	}
	for _, c := range cs {
		uri := c.ResolvedURI()
		bad = append(bad, ambiguousLayerNames(uri+" carries", keys(c.Layers))...)
		d, ok := kinds.Get(c.Type)
		if !ok {
			continue // reported by assertEveryKindIsComplete
		}
		for layer, ref := range c.Layers {
			decl, declared := d.Layers[layer]
			if !declared || decl.MediaType == "" {
				continue
			}
			if !mediaTypeConforms(ref.MediaType, decl.MediaType) {
				bad = append(bad, fmt.Sprintf("  %s: layer %q has media type %q, kind %q declares %q",
					uri, layer, ref.MediaType, c.Type, decl.MediaType))
			}
		}
	}
	sort.Strings(bad)
	return bad
}

func ambiguousLayerNames(who string, names []string) []string {
	var bad []string
	seen := map[string]string{}
	for _, n := range names {
		folded := strings.ToLower(strings.TrimSpace(n))
		if n != folded {
			bad = append(bad, fmt.Sprintf("  %s layer %q: layer names are lower-case (%q)", who, n, folded))
		}
		if prev, dup := seen[folded]; dup {
			bad = append(bad, fmt.Sprintf("  %s layers %q and %q: one name two ways", who, prev, n))
		}
		seen[folded] = n
	}
	return bad
}

// mediaTypeConforms: the carried media type is the declared one, parameters
// aside — or JSON where the kind declares YAML. Every JSON document is a YAML
// 1.2 document, and the schema DSL (the data-type kind's `source`) is authored
// in either; its producer labels a JSON-authored schema honestly as JSON. The
// first run of this guard found exactly that on every sidecar (task
// input/output schemas and config schemas, rendered as JSON), and nothing
// else.
func mediaTypeConforms(carried, declared string) bool {
	c, d := baseMediaType(carried), baseMediaType(declared)
	return c == d || (d == "application/yaml" && c == "application/json")
}

func baseMediaType(mt string) string {
	if mt == "" {
		return ""
	}
	if base, _, err := mime.ParseMediaType(mt); err == nil {
		return base
	}
	return strings.ToLower(strings.TrimSpace(mt))
}

func keys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
