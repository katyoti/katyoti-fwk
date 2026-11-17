// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scripts/api"
)

// timeoutEngine records the bound each run was given.
type timeoutEngine struct{ got map[string]time.Duration }

func (e *timeoutEngine) Name() string         { return "fake" }
func (e *timeoutEngine) Extensions() []string { return []string{".sh"} }
func (e *timeoutEngine) Run(_ context.Context, in api.ScriptExecInput) (api.ScriptRunOutput, error) {
	e.got[in.Name] = in.Timeout
	return api.ScriptRunOutput{}, nil
}

// : a script declares its run bound (scripts.yaml timeout_s)
// the engine applies exactly that, and the script's info carries it for the
// slot method to advertise - a script declaring none gets DefaultTimeout,
// stated, never an engine-private constant.
func TestAScriptRunsAsLongAsItDeclares(t *testing.T) {
	sl := api.ScriptLoader{
		Root:     "scripts",
		Provider: "ktw-docs",
		Loader: loader.NewMapLoader(map[string]string{
			"scripts/scripts.yaml":  "scripts:\n  - name: build-site\n    path: build-site.sh\n    timeout_s: 900\n  - name: quick\n    path: quick.sh\n",
			"scripts/build-site.sh": "echo build",
			"scripts/quick.sh":      "echo quick",
			"scripts/found.sh":      "echo found",
		}),
	}
	eng := &timeoutEngine{got: map[string]time.Duration{}}
	m := &scriptManager{scripts: discoverScripts([]api.ScriptLoader{sl}), engines: map[string]api.ScriptEngine{".sh": eng}}

	want := map[string]time.Duration{
		"ktw-docs:build-site": 900 * time.Second,
		"ktw-docs:quick":      api.DefaultTimeout,
		"ktw-docs:found":      api.DefaultTimeout,
	}
	list, _ := m.List(context.Background())
	if len(list.Scripts) != len(want) {
		t.Fatalf("discovered %d scripts, want %d: %+v", len(list.Scripts), len(want), list.Scripts)
	}
	for _, info := range list.Scripts {
		if got := time.Duration(info.TimeoutS) * time.Second; got != want[info.URI()] {
			t.Errorf("%s advertises %v; want %v", info.URI(), got, want[info.URI()])
		}
	}
	for uri, bound := range want {
		if _, err := m.Run(context.Background(), api.ScriptRunInput{Name: uri}); err != nil {
			t.Fatalf("Run %s: %v", uri, err)
		}
		if eng.got[uri] != bound {
			t.Errorf("%s ran under %v; want its declared %v", uri, eng.got[uri], bound)
		}
	}
}
