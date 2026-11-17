// SPDX-License-Identifier: Apache-2.0

package core

import (
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// TestPublicationTargetBecomesConverter: a planification publication's
// converter, stored as `target`, is kept as `converter`
// another type, or a publication with no target, is left as it is.
func TestPublicationTargetBecomesConverter(t *testing.T) {
	o := &api.Object{Type: "publication", Capabilities: map[string]map[string]any{"publication": {
		"view": "stream", "scope": "KaTyoti", "target": "ktw-example:example/publish-documents",
	}}}
	changed, err := publicationTargetToConverter(o)
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	pub := o.Capabilities["publication"]
	if pub["converter"] != "ktw-example:example/publish-documents" || pub["target"] != nil {
		t.Errorf("publication after the step: %v", pub)
	}
	if changed, _ := publicationTargetToConverter(o); changed {
		t.Error("the step changed a migrated publication again")
	}
	other := &api.Object{Type: "feature", Capabilities: map[string]map[string]any{"publication": {"target": "x"}}}
	if changed, _ := publicationTargetToConverter(other); changed {
		t.Error("the step changed a feature")
	}
}
