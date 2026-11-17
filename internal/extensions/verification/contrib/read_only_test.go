// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	ktwreflect "github.com/katyoti/katyoti-fwk/internal/core/reflect"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/verification/api"
)

// A FUNCTION SAYS WHAT IT DOES. verify() runs every verify hook under the
// object, and an acceptance criterion's hook records its verdict: its method
// is not read-only. lint() only reads. CWM runs a function's action without
// asking it to be read-only only when the function is declared costly — so a
// method that writes must back a costly function.
func TestTheFunctionsSayWhetherTheyWrite(t *testing.T) {
	m := api.VerificationServiceDef.Methods
	if m["VerifyFunction"].ReadOnly {
		t.Error("VerifyFunction is declared read-only, but verify() runs the verify hooks and the criteria's hooks record verdicts")
	}
	if !m["LintFunction"].ReadOnly {
		t.Error("LintFunction only reads the lint answers: it is read-only")
	}

	raw, err := packFS.ReadFile("embed/cwm/capabilities/verification.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var c cwmapi.Capability
	if err := yaml.Unmarshal(raw, &c); err != nil {
		t.Fatal(err)
	}
	if len(c.Functions) != 2 {
		t.Fatalf("the capability declares %d functions, want verify and lint", len(c.Functions))
	}
	for _, f := range c.Functions {
		method := f.Action[strings.LastIndex(f.Action, "/")+1:]
		found := false
		for name, def := range m {
			if ktwreflect.ToKebab(name) != method {
				continue
			}
			found = true
			if !def.ReadOnly && !f.Costly {
				t.Errorf("%s(): its method %s writes, so the function must be costly — CWM only lets a costly function's action write", f.Name, name)
			}
		}
		if !found {
			t.Errorf("%s(): no method %q in the service", f.Name, method)
		}
	}
}
