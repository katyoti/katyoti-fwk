// SPDX-License-Identifier: Apache-2.0

// Package cwmapp boots an Application with a REAL CWMService rooted at a
// temporary project, for tests of the services built over the Object API.
// No fakes on the CWM side: an object a test writes lands on disk and is read
// back through the index, exactly as in a running binary.
package cwmapp

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	busclientbus "github.com/katyoti/katyoti-fwk/internal/appcore/busclient/bus"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	cwmlocal "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/local"
	cwmclient "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/remote"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	manifestcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/contrib"
	manifestlocal "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/local"
	"github.com/katyoti/katyoti-fwk/internal/extensions/manifest/teststub"
	ktslocal "github.com/katyoti/katyoti-fwk/internal/kts/local"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp/messagebus"
	"github.com/katyoti/katyoti-fwk/internal/test/hubharness"
	"github.com/spf13/cobra"
)

// New runs an Application holding the CWM service on t.TempDir() plus exts, and
// returns a context carrying it and the CWM service. No type pack is loaded
// beyond the ones a test contributes, and validation runs in WARN mode: a test
// exercising one extension is not refused for the types of the packs it does
// not load (production runs in error mode).
func New(t testing.TB, exts ...application.Extension) (context.Context, cwmapi.CWMService) {
	t.Helper()
	return NewAt(t, workspaceDir(t), exts...)
}

// NewAt is New on a given workspace root: a second application opened on the
// root of a first (closed) one finds what the first wrote — a restart.
func NewAt(t testing.TB, root string, exts ...application.Extension) (context.Context, cwmapi.CWMService) {
	t.Helper()
	return newAt(t, root, cwmapi.TypeValidationWarn, exts...)
}

// NewWithConfig is New with the configuration files given (name → content,
// e.g. "Planification.yaml"): what application.UseConfig reads.
func NewWithConfig(t testing.TB, files map[string]string, exts ...application.Extension) (context.Context, cwmapi.CWMService) {
	t.Helper()
	return newIn(t, workspaceDir(t), cwmapi.TypeValidationWarn, files, exts...)
}

// NewStrict is New with validation in ERROR mode, as production runs: what a
// declaration refuses is refused, not warned about. The test's packs must then
// declare every type and edge kind it writes.
func NewStrict(t testing.TB, exts ...application.Extension) (context.Context, cwmapi.CWMService) {
	t.Helper()
	return newAt(t, workspaceDir(t), cwmapi.TypeValidationError, exts...)
}

// Members runs an Application whose CWM serves the universe at root/universe
// attaching the member repositories given (cenv.AttachedProjects): one store
// per member behind the store set when there are several, the
// plain store for one. Validation runs in WARN mode, as New.
func Members(t testing.TB, root string, members []string, exts ...application.Extension) (context.Context, cwmapi.CWMService) {
	t.Helper()
	home := application.EnvOf(filepath.Join(root, "universe")).HomeDir()
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	reg := "projects:\n"
	for _, m := range members {
		reg += "  - " + filepath.ToSlash(m) + "\n"
	}
	if err := os.WriteFile(filepath.Join(home, cenv.AttachedProjectsFile), []byte(reg), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cwm := newAt(t, home, cwmapi.TypeValidationWarn, exts...)
	t.Cleanup(func() {
		if c, ok := cwm.(interface{ Close() error }); ok {
			_ = c.Close()
		}
	})
	return ctx, cwm
}

func newAt(t testing.TB, root, mode string, exts ...application.Extension) (context.Context, cwmapi.CWMService) {
	t.Helper()
	return newIn(t, root, mode, nil, exts...)
}

func newIn(t testing.TB, root, mode string, files map[string]string, exts ...application.Extension) (context.Context, cwmapi.CWMService) {
	t.Helper()
	// The manifest extension owns the RefreshHook point cwmlocal contributes
	// to, so it is wired first.
	all := append([]application.Extension{
		// The CLI root every binary has: the catalog names the binary after it.
		// Without it the catalog cannot be built, and the CWM store would see
		// none of the types the test contributes (a Pack).
		func(init *application.ApplicationInit) {
			init.AddContribute("cwmapp", ".root", func(app *application.Application) error {
				application.PutService[appcore.CobraCommand](app, "root", appcore.CobraCommand{Cmd: &cobra.Command{Use: "ktw-test"}})
				return nil
			})
		},
		manifestlocal.Extension(),
		cwmlocal.ExtensionWith(cwmlocal.Options{WorkspaceRoot: root, ObjectTypeValidation: mode}),
	}, exts...)
	app := application.New(config.NewConfigService(loader.NewMapLoader(files)), all...)
	app.SetProvider("ktw-test")
	if err := app.Run(); err != nil {
		t.Fatalf("app.Run: %v", err)
	}
	ctx := application.WithApplication(context.Background(), app)
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		t.Fatalf("resolve CWMService: %v", err)
	}
	return ctx, cwm
}

// Module creates a module object so objects can hang under it: of the test
// world's own `module` type when its pack declares one, else documentation -
// the shipped ktw-methods pack no longer has a module type.
func Module(t testing.TB, ctx context.Context, cwm cwmapi.CWMService, name string) {
	t.Helper()
	var err error
	for _, typ := range []string{"module", "documentation"} {
		obj := cwmapi.NewContainerObject(name, name, typ, "", name, "")
		if _, err = cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: name, Object: obj}); err == nil {
			return
		}
		if !strings.Contains(err.Error(), "undeclared type") {
			break
		}
	}
	t.Fatalf("create module %s: %v", name, err)
}

// Remote runs TWO applications on one in-memory message bus: a host holding the
// real CWM service, published on the bus as ktw-agents publishes it, and a
// consumer holding exts plus the generated bus client as its CWMService — the
// shape of a sidecar that does not host CWM. It returns the consumer's context
// and the HOST's CWM service, so a test acts through the consumer and reads the
// effect where it really landed.
func Remote(t testing.TB, exts ...application.Extension) (context.Context, cwmapi.CWMService) {
	t.Helper()
	return RemoteHosting(t, nil, exts...)
}

// RemoteHosting is Remote with extensions for the HOST too — where CWM runs, so
// a pack its store must consume (types, workflows) goes there.
func RemoteHosting(t testing.TB, host []application.Extension, exts ...application.Extension) (context.Context, cwmapi.CWMService) {
	t.Helper()
	const tenant, universe = "local", "test"
	bus := teststub.New()
	t.Cleanup(func() { _ = bus.Drain(context.Background()) })

	// The host publishes the CWM slot exactly as its KTSP container does (the
	// def's bus name and methods), and nothing else.
	hostCtx, hostCWM := New(t, host...)
	def := cwmapi.CWMServiceDef
	endpoint := ktsp.NewEndpoint(cwmclient.Host, "CWM host", "test")
	endpoint.AddSlot(ktslocal.BuildKTSPSlotWithTags(def.ResolvedBusName(), ktsp.DefaultInstance, def.Short, hostCWM, nil, def.Methods))
	group, err := messagebus.Subscribe(hostCtx, bus, tenant, universe, cwmclient.Host, endpoint)
	if err != nil {
		t.Fatalf("host subscribe: %v", err)
	}
	t.Cleanup(func() { _ = group.Unsubscribe() })

	all := append([]application.Extension{
		cwmclient.RemoteExtension(),
		busclientbus.Extension(), // the bus-backed Sender the generated client calls through
		application.DeclareSimpleService[mbapi.MessageBus](func(context.Context) (mbapi.MessageBus, error) { return bus, nil }),
	}, exts...)
	app := application.New(config.NewConfigService(loader.NewMapLoader(nil)), all...)
	app.SetProvider("ktw-consumer")
	if err := app.Run(); err != nil {
		t.Fatalf("consumer app.Run: %v", err)
	}
	ctx := messagebus.WithScope(application.WithApplication(context.Background(), app), tenant, universe)
	return ctx, hostCWM
}

// Pack contributes a CWM pack to the test application — object types,
// capabilities, workflows — as a sidecar's own pack is contributed, so the CWM
// service consumes it from the catalog like any other — the extensions'
// folders (gates/, gate-requirements/, object-panels/, agent-contexts/)
// included: they are CWM kinds.
func Pack(fsys fs.FS) application.Extension {
	return manifestcontrib.ContributePackPart("", "test-pack", "cwm", fsys)
}

// SystemModules contributes the shipped `system-module` object type and its
// capability (ktw-agents' pack), so a test of an extension that ensures its
// system module (cwmapi.EnsureSystemModule) writes it as production does.
func SystemModules(t testing.TB) application.Extension {
	t.Helper()
	return Pack(fstest.MapFS{
		"capabilities/system-module.yaml": {Data: ShippedFile(t, "app/ktw-agents/embed/cwm/capabilities/system-module.yaml")},
		"object-types/system-module.yaml": {Data: ShippedFile(t, "app/ktw-agents/embed/cwm/object-types/system-module.yaml")},
	})
}

// SoftwareModules contributes the shipped `software-module` object type and the
// `code` capability (ktw-code's pack), so a test of what creates
// software modules (gocode's ModuleCreate) writes them as production does.
func SoftwareModules(t testing.TB) application.Extension {
	t.Helper()
	return Pack(fstest.MapFS{
		"capabilities/code.yaml":            {Data: ShippedFile(t, "app/ktw-code/embed/cwm/capabilities/code.yaml")},
		"object-types/software-module.yaml": {Data: ShippedFile(t, "app/ktw-code/embed/cwm/object-types/software-module.yaml")},
	})
}

// ShippedFile reads a file a sidecar ships (repo-relative, e.g.
// "app/ktw-methods/embed/cwm/workflows/decision.yaml"), so a test's pack can
// carry the shipped definition rather than a copy.
func ShippedFile(t testing.TB, rel string) []byte {
	t.Helper()
	_, here, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(here), "..", "..", "..")
	b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("shipped file %s: %v", rel, err)
	}
	return b
}

// workspaceDir is the test's workspace folder, removed when the test ends —
// retried while a closing application still holds a file (hubharness.TempDir).
func workspaceDir(t testing.TB) string {
	t.Helper()
	return hubharness.TempDir(t)
}
