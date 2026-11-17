// SPDX-License-Identifier: Apache-2.0

package api_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/contribution/datatypegen"
	"github.com/katyoti/katyoti-fwk/internal/extensions/universeview/api"
)

// IN-030's display switch is retired: the
// person approved its delivery, so a tag element names no display switch -
// TagItem has no `with` / `without` and the universe-tag data-type names no
// switch.
func TestATagElementNamesNoDisplaySwitch(t *testing.T) {
	typ := reflect.TypeOf(api.TagItem{})
	for i := 0; i < typ.NumField(); i++ {
		if j := typ.Field(i).Tag.Get("json"); strings.HasPrefix(j, "with") {
			t.Errorf("TagItem.%s still ties an element to a display switch", typ.Field(i).Name)
		}
	}
	dsl, err := datatypegen.Render(api.TagKind)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(dsl), "in030") {
		t.Error("the universe-tag data-type still names the retired display switch")
	}
}
