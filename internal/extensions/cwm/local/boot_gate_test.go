// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	actionslocal "github.com/katyoti/katyoti-fwk/internal/extensions/actions/local"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	manifestcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/contrib"
	manifestlocal "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/local"
	ktsapi "github.com/katyoti/katyoti-fwk/internal/kts/api"
	ktslocal "github.com/katyoti/katyoti-fwk/internal/kts/local"
	"github.com/spf13/cobra"
)

// Ensurer is an inbox-like service of the binary under test: a bootstrap hook
// names its `ensure` method.
type Ensurer interface {
	Ensure(ctx context.Context, in EnsureInput) (EnsureOutput, error)
}

type EnsureInput struct {
	Point string `json:"point,omitempty"`
}

type EnsureOutput struct {
	Ensured bool `json:"ensured"`
}

type recordingEnsurer struct {
	mu    sync.Mutex
	calls []string
}

func (r *recordingEnsurer) Ensure(_ context.Context, in EnsureInput) (EnsureOutput, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, in.Point)
	return EnsureOutput{Ensured: true}, nil
}

func (r *recordingEnsurer) seen() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.calls...)
}

// TestBootstrapHookOnOwnSlotRunsInProcessDuringBoot: a project opens in a
// binary that serves the slot its bootstrap hook names — the shape of
// ktw-agents and inbox/ensure. The KTSP container builds the CWM service at
// AppInit and publishes its endpoint only once it has resolved every service
// (here, a slow one after CWM), so a bootstrap run from the CWM factory found no
// endpoint and went to the bus. The hook must run in-process, during boot,
// with no hub: it has run by the time Run returns.
func TestBootstrapHookOnOwnSlotRunsInProcessDuringBoot(t *testing.T) {
	ensurer, _ := bootWithBootstrapHooks(t, nil)
	if got := ensurer.seen(); len(got) != 1 || got[0] != string(cwmapi.HookBootstrap) {
		t.Fatalf("ensure calls at the end of boot = %v; want the bootstrap hook run once, in-process", got)
	}
}

// TestBootstrapHookOfAnotherSidecarWaitsForServing: a project opens in a
// binary whose bootstrap names a slot ANOTHER sidecar serves — the shape of
// ktw-demo's review/ensure-desk under ktw-agents. Its action calls CWM back over
// the bus, which this process answers only once it serves; run at boot, each
// such call waited out its request timeout, and with a few projects the
// serving process missed the hub's readiness deadline and was killed, again
// and again. Boot runs only the binary's own hooks; the serving hook runs the
// others.
func TestBootstrapHookOfAnotherSidecarWaitsForServing(t *testing.T) {
	logs := &capturedLogs{}
	prev := slog.Default()
	slog.SetDefault(slog.New(logs))
	t.Cleanup(func() { slog.SetDefault(prev) })

	ensurer, app := bootWithBootstrapHooks(t, map[string]string{
		"hooks/zz-other.yaml": "type: bootstrap\nuri: ktw-other:review/ensure-desk\nfilter:\n  types: [project]\n",
	})
	if got := ensurer.seen(); len(got) != 1 {
		t.Fatalf("ensure calls at the end of boot = %v; want this binary's own bootstrap hook run once", got)
	}
	if logs.has("ktw-other:review/ensure-desk") {
		t.Fatalf("the hook another sidecar serves was tried during boot: %v", logs.all())
	}

	ctx := application.WithApplication(context.Background(), app)
	var serving *ktsapi.ServingHook
	for _, h := range application.UseExtensions[ktsapi.ServingHook](ctx) {
		if h.Name == "cwm-bootstrap" {
			serving = &h
		}
	}
	if serving == nil {
		t.Fatal("no cwm-bootstrap serving hook: the hooks other sidecars serve never run")
	}
	// No hub here: the call fails, and its failure names the hook — it ran.
	serving.Start(ctx)
	if !logs.has("ktw-other:review/ensure-desk") {
		t.Fatalf("the serving hook did not run the hook another sidecar serves: %v", logs.all())
	}
	if got := ensurer.seen(); len(got) != 1 {
		t.Fatalf("ensure calls after serving = %v; the serving hook ran this binary's own hook again", got)
	}
}

// bootWithBootstrapHooks runs a binary holding CWM, with a project and a
// bootstrap hook on its own `ensurer/ensure` slot, plus extra pack files.
func bootWithBootstrapHooks(t *testing.T, extra map[string]string) (*recordingEnsurer, *application.Application) {
	t.Helper()
	root := t.TempDir()
	project := "id: demo\ntype: project\nname: demo\ncapabilities:\n  project: {uuid: 0192f0a0-0000-7000-8000-000000000001}\n"
	if err := os.WriteFile(filepath.Join(root, cwmapi.ProjectFileName), []byte(project), 0o644); err != nil {
		t.Fatal(err)
	}
	hookType, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "app", "ktw-agents", "embed", "cwm", "hook-types", "bootstrap.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	files := fstest.MapFS{
		"object-types/project.yaml": {Data: []byte("name: project\n")},
		"hook-types/bootstrap.yaml": {Data: hookType},
		// The production shape (inbox-bootstrap.yaml): the binary's own slot.
		"hooks/zz-ensure.yaml": {Data: []byte("type: bootstrap\nuri: :ensurer/ensure\nfilter:\n  types: [project]\n")},
	}
	for name, data := range extra {
		files[name] = &fstest.MapFile{Data: []byte(data)}
	}
	pack := manifestcontrib.ContributePackPart("", "zz-boot", "cwm", files)
	ensurer := &recordingEnsurer{}
	app := application.New(
		config.NewConfigService(loader.NewMapLoader(nil)),
		func(init *application.ApplicationInit) {
			init.AddContribute("test", ".root", func(app *application.Application) error {
				application.PutService[appcore.CobraCommand](app, "root", appcore.CobraCommand{Cmd: &cobra.Command{Use: "ktw-test"}})
				return nil
			})
		},
		manifestlocal.Extension(),
		ExtensionWith(Options{WorkspaceRoot: root, ObjectTypeValidation: cwmapi.TypeValidationWarn}),
		pack,
		actionslocal.Extension(),
		// Resolved by the container AFTER CWM, and slowly: the endpoint is
		// published well after the CWM factory has returned.
		appcore.PublishSimpleService[Ensurer](func(context.Context) (Ensurer, error) {
			time.Sleep(300 * time.Millisecond)
			return ensurer, nil
		}, appcore.WithKTSPContainer("ensurer")),
		ktslocal.AutoKTSPContainer("ktw-test", "the binary under test", "0"),
	)
	app.SetProvider("ktw-test")
	if err := app.Run(); err != nil {
		t.Fatalf("app.Run: %v", err)
	}
	t.Cleanup(func() { appcore.CloseApplicationScope(application.WithApplication(context.Background(), app)) })
	return ensurer, app
}

// capturedLogs keeps every log record's message and attributes, as one line.
type capturedLogs struct {
	mu    sync.Mutex
	lines []string
}

func (c *capturedLogs) Enabled(context.Context, slog.Level) bool { return true }
func (c *capturedLogs) WithAttrs([]slog.Attr) slog.Handler       { return c }
func (c *capturedLogs) WithGroup(string) slog.Handler            { return c }
func (c *capturedLogs) Handle(_ context.Context, r slog.Record) error {
	line := r.Message
	r.Attrs(func(a slog.Attr) bool { line += " " + a.String(); return true })
	c.mu.Lock()
	c.lines = append(c.lines, line)
	c.mu.Unlock()
	return nil
}

func (c *capturedLogs) all() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.lines...)
}

func (c *capturedLogs) has(s string) bool {
	for _, l := range c.all() {
		if strings.Contains(l, s) {
			return true
		}
	}
	return false
}

// TestBootGateRunsLateWorkAtOnce: a service built after boot (resolved lazily
// at runtime) does not wait for a phase that has passed.
func TestBootGateRunsLateWorkAtOnce(t *testing.T) {
	g := &bootGate{}
	var order []string
	g.afterBoot(func() { order = append(order, "early") })
	if len(order) != 0 {
		t.Fatalf("ran before boot: %v", order)
	}
	g.open()
	if len(order) != 1 {
		t.Fatalf("queued work after open: %v; want it run", order)
	}
	done := make(chan struct{})
	g.afterBoot(func() { close(done) })
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("work queued after boot never ran")
	}
}
