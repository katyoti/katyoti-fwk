// SPDX-License-Identifier: Apache-2.0

package remote_test

import (
	"context"
	"strings"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/manifest/remote"
)

// The remote body lane answers the DATA layer and nothing else — and asks the
// aggregator for nothing when an entry has no data layer.
//
// A data-type carries only a source layer. This reader used to return that
// source as the "body", and, when it was not resident, to ask the hub for it by
// the SOURCE hash through GetBody — so YAML travelled the body lane and reached
// a JSON decoder. Now: nil, and no call.
func TestRemoteGetBodyNeverAnswersSource(t *testing.T) {
	dsl := contribapi.Contribution{Provider: "p", Type: contribapi.ResourceTypeDataType, ID: "shape"}
	dsl.SetSource([]byte("$schema: ktw-schema/v1\nname: shape\n"), "application/yaml")

	sender := &fakeSender{pages: [][]contribapi.Contribution{{dsl}}, bodies: map[string][]byte{}}
	cat := remote.New(sender)

	body, err := cat.GetBody(context.Background(), "p:/data-type/shape")
	if err != nil {
		t.Fatalf("GetBody: %v", err)
	}
	if len(body) != 0 {
		t.Fatalf("the body lane answered the SOURCE layer: %q", body)
	}
	for _, p := range sender.paths {
		if strings.HasSuffix(p, "GetBody") {
			t.Fatalf("asked the aggregator for a body the entry does not have (paths: %v)", sender.paths)
		}
	}
}
