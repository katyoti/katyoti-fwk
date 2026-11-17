// SPDX-License-Identifier: Apache-2.0

package core

import (
	"encoding/json"
	"strings"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
)

// A source.sync payload stays whole JSON within what a raised event carries:
// a long why is cut on a rune boundary, more so beside a long source id.
func TestASyncPayloadStaysWithinARaisedEvent(t *testing.T) {
	why := strings.Repeat("élément illisible ; ", 20)
	for _, src := range []string{"m.apps", "m." + strings.Repeat("a-very-long-source-name-", 5)} {
		raw, err := syncPayload(api.SourceSyncProgress{Source: src, Phase: api.SyncFailed, Read: 2, Total: 3, Error: why})
		if err != nil {
			t.Fatal(err)
		}
		if len(raw) > cwmapi.NoteMax {
			t.Errorf("%s: %d bytes, more than %d", src, len(raw), cwmapi.NoteMax)
		}
		var p api.SourceSyncProgress
		if err := json.Unmarshal(raw, &p); err != nil || !strings.HasSuffix(p.Error, "…") || !strings.HasPrefix(why, strings.TrimSuffix(p.Error, "…")) {
			t.Errorf("%s: not a whole payload with its why cut: %v %s", src, err, raw)
		}
	}
	if raw, _ := syncPayload(api.SourceSyncProgress{Source: "m.apps", Phase: api.SyncEnded, Read: 2, Total: 2}); string(raw) != `{"source":"m.apps","phase":"ended","read":2,"total":2}` {
		t.Errorf("a short payload is kept as is: %s", raw)
	}
}
