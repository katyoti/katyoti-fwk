// SPDX-License-Identifier: Apache-2.0

package census

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"

	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// assertConfigFilesHaveTemplates: every config file a binary's services read
// ships its template (embed/config/<name>), so a fresh universe's config check
// offers the default and `sc config setup` writes it. A file with none is
// reported missing with nothing to write - ktw-sync's template once named
// after the wrong service.
//
// knownMissingTemplates are the drifts found when the guard was added, each
// waiting on the request filed in its owner's module; that request removes
// its line. A new drift fails.
var knownMissingTemplates = map[string]string{}

func assertConfigFilesHaveTemplates(t *testing.T, name string, m *mbapi.Manifest) {
	t.Helper()
	var missing []string
	for _, c := range m.Contributions {
		if c.Type != mbapi.ResourceTypeConfigFile {
			continue
		}
		var body mbapi.ConfigFileBody
		if err := json.Unmarshal(c.Data(), &body); err != nil {
			t.Errorf("census[%s]: %s: read its body: %v", name, c.ResolvedURI(), err)
			continue
		}
		if !body.HasDefault && knownMissingTemplates[name+"/"+c.ID] == "" {
			missing = append(missing, c.ID)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		t.Errorf("census[%s]: config file(s) a service reads with no template in embed/config: %s\n"+
			"  add embed/config/<name> (its defaults, commented) so a fresh universe gets it", name, strings.Join(missing, ", "))
	}
}
