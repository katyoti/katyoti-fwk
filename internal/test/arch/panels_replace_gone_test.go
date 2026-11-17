// SPDX-License-Identifier: Apache-2.0

package arch

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// WHAT DISPLAY PANELS REPLACE DOES NOT COME BACK. A long
// method's progress, its later data and its question reach a screen as a
// display panel's events (sections/panels, internal/core/panel); the polled
// paths they replaced are deleted and named here, so no Go, TypeScript or
// panel source carries one again:
//   - LiveObjectView and the method panel's descriptor-driven live action
//     (start async, poll a progress method, re-render);
//   - render.Def's `live` source and RenderDefView's polling of it;
//   - the run-progress registry a method was polled through;
//   - terminal.ProgressContent, a progress message no producer emitted;
//   - humangate's UIEventAwaiter, the in-memory wait of a form ask
//     (AwaitUIEvent, DeliverUIEvent): ask-user-data asks on a question
//     panel and takes its answer from the panel's input;
//   - the translator path of a chat's UI artifacts: the
//     artifact message (UIArtifactContent, its msg.new `ui_artifact`), the
//     ui.value_patch and ui.complete events and the GUI's own patching of
//     them (lib/patch-apply.ts, lib/id-lookup.ts, the store's applyPatch) -
//     every chat draws a run's artifacts from its chat panel's events. The
//     agent's agent.ui_artifact_mount/patch/complete events stay: they are
//     the tools' one link to the chat panel bridge;
//   - the GUI generic action dispatch's blanket actionRunTimeout
//     : an action run waits as long as its target declares
//     (actionRunBudget), a script its advertised run bound;
//   - IN-038's display switch and the screens it kept: the
//     panels are the one display, so no source reads KTW_IN038_DISPLAY or
//     useDisplaySwitch("in038") in any quoting, and the switch-off paths
//     are gone - the method panel's, the document export's and the object Verify's direct
//     call that waits (actionRoute, exportRoute, verifyRoute, DocumentExport,
//     WorkspaceObjectVerify) and the installs' direct marketplace call
//     (installClient's switch).
var panelsReplace = map[string]*regexp.Regexp{
	"LiveObjectView":                  regexp.MustCompile(`\bLiveObjectView\b`),
	"the live action's handlers":      regexp.MustCompile(`\bdescriptorLiveHandlers\b|\bisLiveAction\b`),
	"render.Def's live source":        regexp.MustCompile(`\brender\.Live\b|\bLive\s+\*Live\b|\bdef\.live\b`),
	"the run-progress registry":       regexp.MustCompile(`run-progress|\bRunProgress\b`),
	"terminal.ProgressContent":        regexp.MustCompile(`\bProgressContent\b`),
	"a live key on a panel's source":  regexp.MustCompile(`(?m)^\s+live:\s*\{\s*method:`),
	"humangate's UIEventAwaiter":      regexp.MustCompile(`\bUIEventAwaiter\b|\bAwaitUIEvent\b|\bDeliverUIEvent\b`),
	"the chat's artifact message":     regexp.MustCompile(`\bUIArtifactContent\b|\bUIArtifactSubmit\b|['"]ui_artifact['"]`),
	"the chat's ui.* artifact events": regexp.MustCompile(`ui\.value_patch|['"]ui\.complete['"]|\bUIPatchEvent\b|\bUICompleteEvent\b|\bNewPatchEvent\b|\bNewCompleteEvent\b`),
	"the GUI's own artifact patching": regexp.MustCompile(`lib/patch-apply|lib/id-lookup|\bapplyPatch\s*[(:]`),
	"the dispatch's blanket ceiling":  regexp.MustCompile(`\bactionRunTimeout\b`),
	"IN-038's display switch":         regexp.MustCompile(`KTW_IN038_DISPLAY|\bIn038DisplayEnv\b|useDisplaySwitch\(\s*['"\x60]in038['"\x60]\s*\)`),
	"the switch-off routes":           regexp.MustCompile(`\bactionRoute\b|\bverifyRoute\b|\bexportRoute\(|installClient\((true|false|displayPanels)\b`),
	"the direct calls that wait":      regexp.MustCompile(`\bDocumentExport\(|\bWorkspaceObjectVerify\(|\bobjectVerifyVia\b`),
}

func TestWhatPanelsReplaceStaysDeleted(t *testing.T) {
	root := repoRoot(t)
	checked := 0
	for _, dir := range []string{"internal", "app"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				switch d.Name() {
				case "node_modules", "dist", "testdata", "_compiled", ".git":
					return filepath.SkipDir
				}
				return nil
			}
			name := d.Name()
			if strings.HasSuffix(name, "_test.go") || strings.Contains(name, ".test.") {
				return nil
			}
			switch filepath.Ext(name) {
			case ".go", ".ts", ".tsx", ".yaml":
			default:
				return nil
			}
			raw, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			checked++
			rel := filepath.ToSlash(strings.TrimPrefix(p, root+string(filepath.Separator)))
			for what, re := range panelsReplace {
				if m := re.FindString(string(raw)); m != "" {
					t.Errorf("%s names %q - %s was deleted: a long method reports into its display panel", rel, m, what)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if checked < 1000 {
		t.Fatalf("checked %d files: the roots are wrong", checked)
	}
}
