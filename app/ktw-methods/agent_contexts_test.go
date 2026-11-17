// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	"github.com/katyoti/katyoti-fwk/internal/core/meta"
	corereflect "github.com/katyoti/katyoti-fwk/internal/core/reflect"
	decisionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/decisions/api"
	featuresapi "github.com/katyoti/katyoti-fwk/internal/extensions/features/api"
	goalsapi "github.com/katyoti/katyoti-fwk/internal/extensions/goals/api"
	ocapi "github.com/katyoti/katyoti-fwk/internal/extensions/objectcontext/api"
	usecasesapi "github.com/katyoti/katyoti-fwk/internal/extensions/usecases/api"
)

// TestShippedAgentContextsNameRealTools: every tool an agent-context of this
// pack hands a chat names a method its service publishes (the kebab wire
// name), and every skill it advertises ships here. A framing that offered a
// tool nobody serves would fail only when an agent called it.
func TestShippedAgentContextsNameRealTools(t *testing.T) {
	methods := map[string]map[string]bool{}
	for _, def := range []meta.ServiceDef{goalsapi.GoalsServiceDef, decisionsapi.DecisionsServiceDef, featuresapi.FeaturesServiceDef, usecasesapi.UseCasesServiceDef} {
		m := map[string]bool{}
		for name := range def.Methods {
			m[corereflect.ToKebab(name)] = true
		}
		methods[def.Name] = m
	}
	dir := filepath.Join("embed", "cwm", "agent-contexts")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) < 3 {
		t.Fatalf("only %d agent-contexts shipped", len(entries))
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".i18n.yaml") {
			continue // an agent context's translations, carried as its layer — not a declaration
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		var f ocapi.AgentFraming
		if err := config.DecodeBytes(raw, marshal.YAML, &f); err != nil {
			t.Fatalf("%s: %v", e.Name(), err)
		}
		if len(f.Agent.Tools) == 0 {
			t.Errorf("%s hands a chat no tools", e.Name())
		}
		for _, tool := range f.Agent.Tools {
			if tool.Plugin != "" {
				continue // a plugin is the host's (verification: object-verify), not a method published here
			}
			svc, method, ok := strings.Cut(strings.TrimPrefix(tool.URI, "ktw-methods:"), "/")
			if !ok || !strings.HasPrefix(tool.URI, "ktw-methods:") || !methods[svc][method] {
				t.Errorf("%s: tool %q is no method a ktw-methods service publishes", e.Name(), tool.URI)
			}
		}
		for _, s := range f.Agent.Skills {
			if _, err := os.Stat(filepath.Join("embed", "skills", s.Selector+".md")); err != nil {
				t.Errorf("%s: skill %q is not shipped", e.Name(), s.Selector)
			}
		}
	}
}
