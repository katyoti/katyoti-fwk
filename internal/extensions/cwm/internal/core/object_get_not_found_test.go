// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"errors"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// An id the index does not know, asked without a module, is NOT FOUND and the
// answer names the id — not "module required", which sent the user hunting for
// a flag when the object had simply moved away or never existed.
func TestObjectGet_UnknownIDWithoutModuleIsNotFound(t *testing.T) {
	root := freshWorkspace(t)
	svc, err := New(stdctx.Background(), &CWMServiceConfig{WorkspaceRoot: root})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = svc.Close() })
	ctx := stdctx.Background()

	obj := sampleObject()
	if _, err := svc.ObjectCreate(ctx, api.ObjectCreateInput{Module: "cwm", Object: obj}); err != nil {
		t.Fatalf("create: %v", err)
	}
	// A known id needs no module: the index finds it.
	if _, err := svc.ObjectGet(ctx, api.ObjectGetInput{ID: obj.ID}); err != nil {
		t.Fatalf("get a known id without module: %v", err)
	}

	for _, id := range []string{"cwm.never-was", obj.ID} {
		if id == obj.ID {
			if _, err := svc.ObjectDelete(ctx, api.ObjectDeleteInput{Module: "cwm", ID: obj.ID}); err != nil {
				t.Fatalf("delete: %v", err)
			}
		}
		_, err := svc.ObjectGet(ctx, api.ObjectGetInput{ID: id})
		if !errors.Is(err, api.ErrNotFound) {
			t.Errorf("get %q without module: err = %v, want ErrNotFound", id, err)
			continue
		}
		if !strings.Contains(err.Error(), id) {
			t.Errorf("not-found message %q does not name %q", err, id)
		}
	}
}
