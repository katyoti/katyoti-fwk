// SPDX-License-Identifier: Apache-2.0

package main

import (
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	kreflect "github.com/katyoti/katyoti-fwk/internal/core/reflect"
	"github.com/katyoti/katyoti-fwk/internal/extensions/compose/api"
)

// The marketplace-project node is the one surface to author a whole
// marketplace: its fields say where the definition is, and its actions Check,
// Publish and Start over run compose's build-from-definition on them (Check a
// dry run, Start over a reinit), Promote commits the published tree. Every
// action names a compose method, every input it binds is that method's, and
// every field it reads is one the node's data-type declares.
func TestTheMarketplaceProjectNodeAuthorsAWholeMarketplace(t *testing.T) {
	var ot struct {
		Fields  string `yaml:"fields"`
		Actions []struct {
			URI   string         `yaml:"uri"`
			Label string         `yaml:"label"`
			Input map[string]any `yaml:"input"`
		} `yaml:"actions"`
	}
	decode(t, "embed/cwm/object-types/marketplace-project.yaml", &ot)
	if ot.Fields != "ktw-compose:/data-type/marketplace-project" {
		t.Fatalf("fields = %q, want the marketplace-project data-type", ot.Fields)
	}
	var dt struct {
		Types map[string]struct {
			Fields map[string]any `yaml:"fields"`
		} `yaml:"types"`
	}
	decode(t, "embed/cwm/data-types/marketplace-project.dsl.yaml", &dt)
	fields := dt.Types["MarketplaceProject"].Fields
	if _, ok := fields["definition"]; !ok {
		t.Fatalf("the data-type declares no definition field: %v", fields)
	}

	svc := reflect.TypeOf((*api.ComposeService)(nil)).Elem()
	methods := map[string]reflect.Type{}
	for i := 0; i < svc.NumMethod(); i++ {
		m := svc.Method(i)
		methods["ktw-compose:compose/"+kreflect.ToKebab(m.Name)] = m.Type.In(1)
	}
	ref := regexp.MustCompile(`^\$\{object\.fields\.([a-zA-Z]+)\}$`)
	byLabel := map[string]map[string]any{}
	for _, a := range ot.Actions {
		in, ok := methods[a.URI]
		if !ok {
			t.Errorf("%s: %s is no compose method", a.Label, a.URI)
			continue
		}
		byLabel[a.Label] = a.Input
		for k, v := range a.Input {
			if !hasJSONField(in, k) {
				t.Errorf("%s: input %q is not a field of %s", a.Label, k, in.Name())
			}
			if s, ok := v.(string); ok && strings.HasPrefix(s, "${") {
				m := ref.FindStringSubmatch(s)
				if m == nil {
					t.Errorf("%s: input %q binds %q, not a field of the node", a.Label, k, s)
				} else if _, ok := fields[m[1]]; !ok {
					t.Errorf("%s: input %q reads field %q the data-type does not declare", a.Label, k, m[1])
				}
			}
		}
	}
	for _, label := range []string{"Check", "Publish", "Start over", "Promote"} {
		if _, ok := byLabel[label]; !ok {
			t.Errorf("no %s action on the node", label)
		}
	}
	if byLabel["Check"]["dryRun"] != true {
		t.Errorf("Check is not a dry run: %v", byLabel["Check"])
	}
	if byLabel["Start over"]["reinit"] != true {
		t.Errorf("Start over does not reinit: %v", byLabel["Start over"])
	}
	if byLabel["Publish"]["dryRun"] == true || byLabel["Publish"]["reinit"] == true {
		t.Errorf("Publish is a dry run or a reinit: %v", byLabel["Publish"])
	}
	for _, label := range []string{"Check", "Publish", "Start over"} {
		if byLabel[label]["definition"] != "${object.fields.definition}" {
			t.Errorf("%s does not read the node's definition: %v", label, byLabel[label])
		}
	}
}

func decode(t *testing.T, path string, v any) {
	t.Helper()
	data, err := embeddedFS.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := config.DecodeBytes(data, marshal.YAML, v); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
}

func hasJSONField(t reflect.Type, name string) bool {
	for i := 0; i < t.NumField(); i++ {
		if strings.Split(t.Field(i).Tag.Get("json"), ",")[0] == name {
			return true
		}
	}
	return false
}
