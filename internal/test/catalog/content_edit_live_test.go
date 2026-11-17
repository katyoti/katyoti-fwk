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
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp/messagebus"
	"github.com/katyoti/katyoti-fwk/internal/test/hubharness"
)

// content_edit_live_test.go — the files-only authoring loop, end to end.
//
// An author edits a file in a served folder and a consumer sees the new
// content, with nothing restarted. That is the whole point of live sources, and
// it is the case that failed: the staleness gate watched the host EXECUTABLE,
// which is identical whatever folder it serves, so nothing ever re-probed.
//
// Skips cleanly when the binaries are not built.

// contentUniverse is the kebab-case universe token. Subject builders panic on
// anything else, so it is pinned rather than derived from the test's name.
const contentUniverse = "content-edit-live"

// makeContentWorkspace writes a workspace whose membership has one FOLDER
// member, declaring its content root.
func makeContentWorkspace(t *testing.T, packName, folder string) string {
	t.Helper()
	ws := t.TempDir()
	cfgDir := filepath.Join(ws, ".katyoti", "config")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := fmt.Sprintf(`version: 1
name: %s
display_name: Content Edit Live
icon: T
color: gray
sidecars:
  - name: %s
    content_root: %s
`, contentUniverse, packName, filepath.ToSlash(folder))
	if err := os.WriteFile(filepath.Join(cfgDir, "Univers.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "Settings.yaml"), []byte("# content edit live test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return ws
}

// contentPack writes a minimal files-only pack and returns its folder.
func contentPack(t *testing.T, name, skillBody string) string {
	t.Helper()
	dir := t.TempDir()
	manifest := fmt.Sprintf("name: %s\nversion: 1.0.0\ndescription: a live pack\n", name)
	if err := os.WriteFile(filepath.Join(dir, "ktw-host.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	skillDir := filepath.Join(dir, "skills", "live-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeSkill(t, skillDir, skillBody)
	return dir
}

func writeSkill(t *testing.T, skillDir, body string) {
	t.Helper()
	md := "---\nname: live-skill\ndescription: a skill served from a folder\n---\n\n" + body + "\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestServedFolderEdit_LiveConverges is the acceptance test: edit a file in a
// served folder, and the universe view converges with nothing restarted.
func TestServedFolderEdit_LiveConverges(t *testing.T) {
	const packName = "live-pack"
	folder := contentPack(t, packName, "the original instructions")

	// Membership DECLARES the content root. That declaration is what lets the
	// aggregator fingerprint the folder without asking the member anything.
	ws := makeContentWorkspace(t, packName, folder)
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
		t.Skipf("live content edit: no hub reachable: %v", err)
	}
	defer hc.Close()
	sender := messagebus.NewSender(hc.MessageBus(), tenantID, contentUniverse)
	uri := packName + ":/skill/live-skill"

	// The pack's content is visible at all — which already requires the probe
	// to have been told which folder to serve.
	before := readBody(t, sender, uri, 20*time.Second)
	if !strings.Contains(before, "the original instructions") {
		t.Fatalf("served pack's skill body = %q, want the original instructions", before)
	}

	// An author edits the file. Nothing is restarted, and the host binary is
	// untouched.
	writeSkill(t, filepath.Join(folder, "skills", "live-skill"), "the revised instructions")
	after := pollForBody(t, sender, uri, "the revised instructions", 30*time.Second)
	if !strings.Contains(after, "the revised instructions") {
		t.Fatalf("after editing a served file, the universe still serves %q — "+
			"the edit never converged", after)
	}
}

// readBody fetches one contribution's body through the aggregator, retrying
// while the universe hydrates.
func readBody(t *testing.T, sender *messagebus.Sender, uri string, within time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(within)
	var lastErr error
	for time.Now().Before(deadline) {
		body, err := tryBodyErr(sender, uri)
		if body != "" {
			return body
		}
		if err != nil {
			lastErr = err
		}
		time.Sleep(500 * time.Millisecond)
	}
	if lastErr != nil {
		t.Fatalf("read %s: never returned content within %s; last error: %v", uri, within, lastErr)
	}
	return ""
}

// pollForBody waits for the served body to contain `want`.
func pollForBody(t *testing.T, sender *messagebus.Sender, uri, want string, within time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(within)
	last := ""
	for time.Now().Before(deadline) {
		last = tryBody(sender, uri)
		if strings.Contains(last, want) {
			return last
		}
		time.Sleep(500 * time.Millisecond)
	}
	return last
}

// tryBody asks the universe for the CONTENT a reader would see.
//
// That is the Read lane, not GetBody. The two answer different questions and
// this test wants the second: GetBody returns the index's descriptor body,
// while Read returns the bytes a consumer renders — a skill's markdown here.
// They used to be the same thing, because a skill inlined its markdown INTO
// the body; c19 moved it to the source layer to stop the index carrying 7 KB
// per skill, and this call kept asking the descriptor for content it no longer
// holds, getting `{}` back.
func tryBody(sender *messagebus.Sender, uri string) string {
	body, _ := tryBodyErr(sender, uri)
	return body
}

// tryBodyErr is tryBody with the reason kept. A poller that discards its error
// reports "" on a transport failure and "" on a genuinely empty body, so a
// broken call reads exactly like a pending one — and the test that times out
// on it can say nothing about why.
func tryBodyErr(sender *messagebus.Sender, uri string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	raw, err := sender.Call(ctx, "ktw-hub:/CatalogService/Read", mbapi.CatalogReadInput{URI: uri})
	if err != nil {
		return "", err
	}
	var out mbapi.CatalogReadOutput
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", err
	}
	return string(out.Content), nil
}
