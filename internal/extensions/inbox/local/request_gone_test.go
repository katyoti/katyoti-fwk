// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"errors"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
)

// A request deleted from the inbox is gone: reading it says so (ErrGone), so
// a task waiting on it stops waiting instead of waiting for ever.
func TestADeletedRequestIsGone(t *testing.T) {
	ctx, cwm, svc := setup(t)
	filed, err := svc.File(ctx, api.FileInput{Title: "Deploy?", RequestedBy: "run-1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Get(ctx, api.GetInput{ID: filed.Request.ID}); err != nil {
		t.Fatalf("a filed request reads: %v", err)
	}
	if _, err := cwm.ObjectDelete(ctx, cwmapi.ObjectDeleteInput{Module: "sys-inbox", ID: filed.Request.ID, Detach: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Get(ctx, api.GetInput{ID: filed.Request.ID}); !errors.Is(err, api.ErrGone) {
		t.Errorf("reading a deleted request: %v; want ErrGone", err)
	}
}
