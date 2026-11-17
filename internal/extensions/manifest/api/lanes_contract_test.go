// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"errors"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
)

// Self reach refuses a layer the entry does not declare with
// contribapi.ErrLayerNotDeclared, before asking the producer anything (e14;
// universe reach is lanes_contract_test.go in manifest/internal/core) — the
// source layer of a data-only entry included.
func TestOpenSelf_UndeclaredLayerIsAnError(t *testing.T) {
	skill := contribapi.Contribution{Provider: "p", Type: "skill", ID: "doc-create"}
	skill.SetData([]byte(`{"name":"doc-create"}`))
	svc := &stubResourceService{index: []contribapi.Contribution{skill}}

	for _, layer := range []string{"thumbnail", contribapi.LayerSource} {
		_, err := OpenVia(context.Background(), svc, "p:/skill/doc-create", layer)
		var undeclared contribapi.ErrLayerNotDeclared
		if !errors.As(err, &undeclared) || undeclared.Layer != layer {
			t.Fatalf("self reach, layer %q: err = %v, want contribapi.ErrLayerNotDeclared", layer, err)
		}
	}
	if svc.reads != 0 {
		t.Fatalf("an undeclared layer reached the producer (%d calls)", svc.reads)
	}
}
