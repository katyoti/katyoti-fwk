// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/appcore/hubclient"
	"github.com/katyoti/katyoti-fwk/internal/extensions/scenario/api"
)

// A case command run from another universe's process (a walk a verification
// runs from the development universe's ktw-methods) inherits that process's
// hub owner, bus key, bus, tenant and universe. The case's hub must not: owned,
// it ended once no client was connected and the walk's start waited 60s for a
// hub that had hidden its lockfile.
func TestACaseHubStartsWithoutAnotherUniversesIdentity(t *testing.T) {
	t.Setenv(hubclient.OwnerEnv, "4242")
	t.Setenv("KTW_BUS_KEY", `C:\elsewhere\hub\keys\sidecar-ktw-methods.key`)
	t.Setenv("KTW_NATS_URL", "nats://127.0.0.1:4222")
	t.Setenv("KTW_TENANT_ID", "other")
	t.Setenv("KTW_UNIVERSE_ID", "dev-id")
	t.Setenv("KTW_UNIVERSE", "dev")
	t.Setenv("KTW_DATA", `C:\elsewhere\data`)
	t.Setenv("KTW_HOME", `C:\elsewhere`)
	t.Setenv("KTW_TEST_DISPLAY_SWITCH", "1")

	env := caseProcessEnv(`C:\case\universe`, `C:\case\data`)
	got := map[string][]string{}
	for _, kv := range env {
		if i := strings.IndexByte(kv, '='); i > 0 {
			got[strings.ToUpper(kv[:i])] = append(got[strings.ToUpper(kv[:i])], kv[i+1:])
		}
	}
	for _, name := range []string{hubclient.OwnerEnv, "KTW_BUS_KEY", "KTW_NATS_URL", "KTW_TENANT_ID", "KTW_UNIVERSE_ID", "KTW_UNIVERSE", "KTW_HOME"} {
		if v, ok := got[name]; ok {
			t.Errorf("the case's hub inherits %s=%v from the process that ran the command", name, v)
		}
	}
	if v := got["KTW_DATA"]; len(v) != 1 || v[0] != `C:\case\data` {
		t.Errorf("KTW_DATA = %v, want the case's data only", v)
	}
	if v := got["KTW_WORKSPACE"]; len(v) != 1 || !strings.HasSuffix(v[0], "universe") {
		t.Errorf("KTW_WORKSPACE = %v, want the case's universe", v)
	}
	if v := got["KTW_TEST_DISPLAY_SWITCH"]; len(v) != 1 || v[0] != "1" {
		t.Errorf("a switch the walk exports is lost: %v", v)
	}
}

// A hub that never answers is reported with why: the 60s wait used to say only
// that it did not answer.
func TestReachSaysWhyTheHubDoesNotAnswer(t *testing.T) {
	e := NewEngine(&api.Case{Name: "demo"}, t.TempDir(), t.TempDir(), nil)
	err := e.reach(context.Background())
	if err == nil {
		t.Fatal("a case with no hub answers")
	}
	if e.Running(context.Background()) {
		t.Fatal("Running with no hub")
	}
}
