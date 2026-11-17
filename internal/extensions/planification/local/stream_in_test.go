// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// streamIn answers a stream of module for an increment to be created in - an
// increment is always created in its stream: the
// first stream the module has, else a new one. An empty module is
// sys-planification, given to the project first.
func streamIn(t *testing.T, ctx context.Context, module string) string {
	t.Helper()
	if module == "" {
		module = api.SystemModule
		svc, err := application.UseService[api.RequestService](ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := svc.Ensure(ctx, api.EnsureInput{}); err != nil {
			t.Fatal(err)
		}
	}
	sm := streams(t, ctx)
	have, err := sm.List(ctx, api.ListStreamsInput{Module: module})
	if err != nil {
		t.Fatal(err)
	}
	if len(have.Streams) > 0 {
		return have.Streams[0].ID
	}
	out, err := sm.Create(ctx, api.CreateStreamInput{Module: module, Name: "Work",
		StreamFields: api.StreamFields{VersionScheme: api.SchemeSemver}})
	if err != nil {
		t.Fatal(err)
	}
	return out.Stream.ID
}

// strayIncrement stores an open increment in no stream, as older data does:
// increments create refuses one, so the guards for
// such data - the « No stream » group, the increment-no-stream lint - are
// tested on one written through cwm.
func strayIncrement(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, module, title string) string {
	t.Helper()
	o := cwmapi.Object{Type: api.IncrementType, Parent: module, Name: title}
	if err := o.SetCapability(api.CapRelease, api.ReleaseFields{}); err != nil {
		t.Fatal(err)
	}
	o.Capabilities[api.CapNetworkNode] = map[string]any{}
	out, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: module, Object: o})
	if err != nil {
		t.Fatal(err)
	}
	return out.Object.ID
}
