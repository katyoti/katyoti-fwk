// SPDX-License-Identifier: Apache-2.0

package machine

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/extensions/scenario/api"
	"github.com/katyoti/katyoti-fwk/internal/test/long"
)

// readyPage stands for the GUI's Ready to start screen: tabs by element id,
// each with its data-state; the profile tab turns ok once a name is typed.
const readyPage = `<!doctype html><html><head><title>KaTyoti - archifactory</title>
<style>body{font:16px sans-serif;margin:0} .tab{display:inline-block;padding:12px;margin:8px;border:2px solid #333}
.ok{background:#2b8a3e;color:#fff} .todo{background:#e03131;color:#fff} p{margin:8px} .band{height:40px;background:linear-gradient(90deg,#1971c2,#f08c00,#7048e8)}</style></head>
<body data-element="screen:ready"><div class="band"></div>
<div class="tab todo" data-element="tab:ready/profile" data-state="todo">Profil et premiers pas</div>
<div class="tab ok" data-element="tab:ready/programs" data-state="ok">Programmes</div>
<div class="tab todo" data-element="tab:ready/keys" data-state="todo">Clés</div>
<p><input data-element="field:ready/name" placeholder="Nom" value="John Doe"></p>
<p><button data-element="action:ready/profile" onclick="save()">Enregistrer</button></p>
<p data-element="text:ready/hello"></p>
<p>` + "Lorem ipsum dolor sit amet, consectetur adipiscing elit. " + `</p><div class="band"></div>
<script>function save(){ const n=document.querySelector('[data-element="field:ready/name"]').value;
 setTimeout(()=>{ const t=document.querySelector('[data-element="tab:ready/profile"]'); t.dataset.state='ok'; t.className='tab ok';
 document.querySelector('[data-element="text:ready/hello"]').innerText='Bonjour ['+n+']' }, 300) }</script></body></html>`

// testBrowser is a Chromium the test can drive: KTW_TEST_CHROME, else Edge or
// Chrome where they install.
func testBrowser(t *testing.T) string {
	if p := os.Getenv("KTW_TEST_CHROME"); p != "" {
		return p
	}
	var candidates []string
	if runtime.GOOS == "windows" {
		for _, base := range []string{os.Getenv("ProgramFiles(x86)"), os.Getenv("ProgramFiles"), os.Getenv("LOCALAPPDATA")} {
			if base != "" {
				candidates = append(candidates, filepath.Join(base, "Microsoft", "Edge", "Application", "msedge.exe"), filepath.Join(base, "Google", "Chrome", "Application", "chrome.exe"))
			}
		}
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	for _, n := range []string{"chromium", "chromium-browser", "google-chrome", "microsoft-edge"} {
		if p, err := exec.LookPath(n); err == nil {
			return p
		}
	}
	t.Skip("no Chromium to drive (set KTW_TEST_CHROME)")
	return ""
}

func freePort(t *testing.T) int {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// openGUI serves the page and opens it in a headless browser with its
// debugging port, as the proof's GUI is started; the port it listens on.
func openGUI(t *testing.T) int {
	browser := testBrowser(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, readyPage)
	}))
	t.Cleanup(srv.Close)
	port := freePort(t)
	// Not t.TempDir: the browser's helpers hold its profile a moment after
	// they are killed, and a TempDir that cannot be removed fails the test.
	profile, err := os.MkdirTemp("", "ktw-gui-stage-")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(browser, "--headless=new", fmt.Sprintf("--remote-debugging-port=%d", port),
		"--user-data-dir="+profile, "--no-first-run", "--no-default-browser-check", "--window-size=1000,700", srv.URL)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start %s: %v", browser, err)
	}
	// The browser's helper processes outlive its main one: the tree goes.
	t.Cleanup(func() {
		if runtime.GOOS == "windows" {
			_ = exec.Command("taskkill", "/F", "/T", "/PID", fmt.Sprint(cmd.Process.Pid)).Run()
		}
		_ = cmd.Process.Kill()
		for i := 0; i < 20 && os.RemoveAll(profile) != nil; i++ {
			time.Sleep(250 * time.Millisecond)
		}
	})
	awaitLoaded(t, port)
	return port
}

// awaitLoaded waits for the served page to be listed with its title, as the
// proof's GUI is once the Ready screen was read.
func awaitLoaded(t *testing.T, port int) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/json/list", port)); err == nil {
			var targets []struct{ Type, URL, Title string }
			_ = json.NewDecoder(resp.Body).Decode(&targets)
			resp.Body.Close()
			for _, tg := range targets {
				if tg.Type == "page" && strings.HasPrefix(tg.URL, "http://") && strings.Contains(tg.Title, "archifactory") {
					return
				}
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatal("the test page did not load")
}

func guiRun(t *testing.T, port int, steps []api.GUIStep, extra func(*api.MachineStage)) StageResult {
	t.Helper()
	results := t.TempDir()
	s := api.MachineStage{ID: "m11-ready-tabs", GUI: &api.GUIStage{Port: port, Steps: steps}, TimeoutMin: 2}
	if extra != nil {
		extra(&s)
	}
	plan := Plan{Case: "proof", Kind: api.MachineLocal, Stages: []api.MachineStage{s}}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	res, err := Run(ctx, plan, World{Home: t.TempDir()}, results, nil)
	if err != nil {
		t.Fatal(err)
	}
	return res.Stages[0]
}

// The walk a proof makes on the Ready screen: the title, a red tab, the
// profile typed and saved, the tab green, a screenshot checked by its pixels.
func TestGUIStageWalksTheScreenByElementIDs(t *testing.T) {
	long.Test(t)
	port := openGUI(t)
	shot := filepath.Join(t.TempDir(), "ready-profile.png")
	sr := guiRun(t, port, []api.GUIStep{
		{Title: "archifactory"},
		{Wait: "tab:ready/profile", State: "todo"},
		{Click: "tab:ready/profile"},
		{Type: "field:ready/name", Text: "Proof", Clear: true},
		{Click: "action:ready/profile"},
		{Wait: "tab:ready/profile", State: "ok"},
		{Wait: "text:ready/hello", Contains: "Bonjour [Proof]"},
		// Without clear, the text goes after what the field holds.
		{Type: "field:ready/name", Text: " again"},
		{Click: "action:ready/profile"},
		{Wait: "text:ready/hello", Contains: "Bonjour [Proof again]"},
		{Shot: shot},
	}, func(s *api.MachineStage) { s.Contains = []string{"gui: tab:ready/profile is ok"} })
	if sr.Status != Passed {
		t.Fatalf("stage %s: %s\n%s", sr.Status, sr.Failure, sr.Output)
	}
	if err := CheckImage(shot); err != nil {
		t.Fatal(err)
	}
}

// Each way a walk goes wrong fails the stage, saying what it last saw.
func TestGUIStageFailsWhatDoesNotHold(t *testing.T) {
	long.Test(t)
	port := openGUI(t)
	cases := map[string]struct {
		steps []api.GUIStep
		want  string
	}{
		"a tab that stays red": {[]api.GUIStep{{Wait: "tab:ready/keys", State: "ok", TimeoutSec: 2}}, `tab:ready/keys is "todo"`},
		"an element not drawn": {[]api.GUIStep{{Click: "tab:ready/nowhere", TimeoutSec: 2}}, "no element tab:ready/nowhere"},
		"a text never shown":   {[]api.GUIStep{{Wait: "text:ready/hello", Contains: "Bonjour", TimeoutSec: 2}}, "text:ready/hello shows"},
		"another window title": {[]api.GUIStep{{Title: "demo", TimeoutSec: 2}}, `the title is "KaTyoti - archifactory"`},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			sr := guiRun(t, port, c.steps, nil)
			if sr.Status != Failed || !strings.Contains(sr.Failure, c.want) {
				t.Fatalf("got %s %q, want failed with %q", sr.Status, sr.Failure, c.want)
			}
		})
	}
	t.Run("no GUI on the port", func(t *testing.T) {
		sr := guiRun(t, freePort(t), []api.GUIStep{{Title: "x"}}, func(s *api.MachineStage) {})
		if sr.Status != Failed || !strings.Contains(sr.Failure, "KTW_GUI_DEBUG_PORT") {
			t.Fatalf("got %s %q", sr.Status, sr.Failure)
		}
	})
}

// A gui stage is checked when the case is read: one kind per stage, one
// action per step.
func TestGUIStageIsValidated(t *testing.T) {
	bad := map[string]api.GUIStep{
		"two actions":        {Click: "a", Wait: "b"},
		"none":               {},
		"state without wait": {Click: "a", State: "ok"},
		"type without text":  {Type: "a"},
		"text without type":  {Click: "a", Text: "x"},
	}
	for name, st := range bad {
		if _, err := st.Kind(); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	for _, st := range []api.GUIStep{{Title: "t"}, {Click: "a"}, {Type: "a", Text: "x"}, {Wait: "a", State: "ok", Contains: "c"}, {Shot: "p.png"}} {
		if _, err := st.Kind(); err != nil {
			t.Errorf("%+v: %v", st, err)
		}
	}
}
