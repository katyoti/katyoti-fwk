// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/appcore/hubclient"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp/messagebus"
	"github.com/katyoti/katyoti-fwk/internal/test/hubharness"
)

// served_yaml_test.go — a folder of YAML files IS a set of contributions, and
// editing one updates what the universe serves.
//
// The sibling content_edit_live_test covers a skill, which is markdown. This
// covers the YAML kinds, and a data-type specifically, because that is the kind
// whose content is a SCHEMA: the thing a consumer resolves to render a form. A
// schema that goes stale is worse than a body that does — the form keeps
// rendering, with the wrong fields.

const yamlUniverse = "served-yaml-live"

// dataTypeDSL is a minimal, valid ktw-schema document. `field` is what the test
// edits, so a converged read is unambiguous.
func dataTypeDSL(field string) string {
	return fmt.Sprintf(`$schema: ktw-schema/v1
name: served-record
version: "1.0.0"
root: ServedRecord
description: a data-type served from a live folder
types:
  ServedRecord:
    type: object
    fields:
      %s:
        type: string
        description: the field under test
`, field)
}

// yamlPack writes a folder holding one YAML data-type and returns it.
func yamlPack(t *testing.T, name, field string) string {
	t.Helper()
	dir := t.TempDir()
	manifest := fmt.Sprintf("name: %s\nversion: 1.0.0\ndescription: a yaml pack\n", name)
	if err := os.WriteFile(filepath.Join(dir, "ktw-host.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	dtDir := filepath.Join(dir, "data-types")
	if err := os.MkdirAll(dtDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeDataType(t, dtDir, field)
	return dir
}

func writeDataType(t *testing.T, dtDir, field string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dtDir, "served-record.yaml"), []byte(dataTypeDSL(field)), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestServedYAML_IsAContributionAndConverges is the acceptance test for
// "a folder of YAML files is a set of contributions, and an edit propagates".
//
// Three assertions, in the order a user meets them:
//  1. the YAML file IS a contribution — it appears in the universe catalog
//     under a resolvable URI, with no restart and no per-kind wiring;
//  2. its CONTENT is readable — a consumer can fetch the schema it carries;
//  3. an EDIT converges — the cache serves the new content, nothing restarted.
func TestServedYAML_IsAContributionAndConverges(t *testing.T) {

	const packName = "yaml-pack"
	folder := yamlPack(t, packName, "before_edit")

	ws := makeYAMLWorkspace(t, packName, folder)
	prevWS, hadWS := os.LookupEnv("KTW_WORKSPACE")
	if err := os.Setenv("KTW_WORKSPACE", ws); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if hadWS {
			_ = os.Setenv("KTW_WORKSPACE", prevWS)
		} else {
			_ = os.Unsetenv("KTW_WORKSPACE")
		}
	})
	dataDir := filepath.Join(ws, ".katyoti")
	hubharness.Start(t, dataDir)

	hc, err := hubclient.ConnectToHub(context.Background(), hubclient.ConnectOptions{
		DataDir: dataDir, Mode: hubclient.ModeReuse,
	})
	if err != nil {
		t.Skipf("served yaml: no hub reachable: %v", err)
	}
	defer hc.Close()
	sender := messagebus.NewSender(hc.MessageBus(), tenantID, yamlUniverse)
	uri := packName + ":/data-type/served-record"

	// (1) The YAML file is a contribution the universe advertises.
	assertListed(t, sender, uri, 30*time.Second)

	// (2) Its content is readable — the schema, not a descriptor.
	before := readBody(t, sender, uri, 20*time.Second)
	if !strings.Contains(before, "before_edit") {
		t.Fatalf("served data-type body = %q, want the authored field", before)
	}
	if !strings.Contains(before, "ktw-schema/v1") {
		t.Errorf("the content read back is not the DSL document the file holds: %q", before)
	}

	// (3) An author edits the YAML. Nothing is restarted.
	writeDataType(t, filepath.Join(folder, "data-types"), "after_edit")
	after := pollForBody(t, sender, uri, "after_edit", 30*time.Second)
	if !strings.Contains(after, "after_edit") {
		t.Fatalf("after editing a served YAML file the universe still serves %q — "+
			"the edit never converged", after)
	}
	if strings.Contains(after, "before_edit") {
		t.Errorf("the pre-edit field survived the refresh: %q", after)
	}
}

// makeYAMLWorkspace is makeContentWorkspace for this test's universe token.
func makeYAMLWorkspace(t *testing.T, packName, folder string) string {
	t.Helper()
	ws := t.TempDir()
	cfgDir := filepath.Join(ws, ".katyoti", "config")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := fmt.Sprintf(`version: 1
name: %s
display_name: Served YAML Live
icon: Y
color: gray
sidecars:
  - name: %s
    content_root: %s
`, yamlUniverse, packName, filepath.ToSlash(folder))
	if err := os.WriteFile(filepath.Join(cfgDir, "Univers.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "Settings.yaml"), []byte("# served yaml test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return ws
}

// assertListed waits for a URI to appear in the universe catalog's INDEX.
//
// Separate from reading its body on purpose: "the catalog knows about it" and
// "its content is reachable" are different failures, and a test that only reads
// cannot tell a missing contribution from an unreadable one.
func assertListed(t *testing.T, sender *messagebus.Sender, uri string, within time.Duration) {
	t.Helper()
	deadline := time.Now().Add(within)
	var lastErr error
	var seen []string
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		raw, err := sender.Call(ctx, "ktw-hub:/CatalogService/ListContributions",
			mbapi.CatalogListContributionsInput{
				Filter: contribapi.ListFilter{Types: []string{contribapi.ResourceTypeDataType}},
			})
		cancel()
		if err != nil {
			lastErr = err
			time.Sleep(500 * time.Millisecond)
			continue
		}
		var out mbapi.CatalogListContributionsOutput
		if err := json.Unmarshal(raw, &out); err != nil {
			lastErr = err
			time.Sleep(500 * time.Millisecond)
			continue
		}
		seen = seen[:0]
		for _, c := range out.Contributions {
			seen = append(seen, c.ResolvedURI())
			if c.ResolvedURI() == uri {
				return
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("%s never appeared in the universe catalog within %s (last error: %v)\n"+
		"data-types the catalog DOES hold: %v", uri, within, lastErr, seen)
}
