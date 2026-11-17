// SPDX-License-Identifier: Apache-2.0

package permission

import (
	"context"
	"errors"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/cause"
	"github.com/katyoti/katyoti-fwk/internal/extensions/profile/api"
)

// : every act's permission code is derived from what it is, so most
// declarations are free.
func TestEveryKindOfActResolvesItsDefaultPermissionCode(t *testing.T) {
	for _, c := range []struct {
		got  api.PermissionCode
		want api.PermissionCode
	}{
		{api.HumanTaskCode("release", "approve"), "release:approve"},
		{api.HumanTaskCode("", "decide"), "universe:decide"},
		{api.HumanTaskCode("Feature", "test-by-hand"), "feature:test-by-hand"},
		{api.MethodCode("features", "Add"), "features:add"},
		{api.MethodCode("todos", "ModuleUpdate"), "todos:module-update"},
		{api.TaskCode("intake"), "task:run:intake"},
		{api.ToolCode("file_write"), "tool:file-write"},
	} {
		if c.got != c.want {
			t.Errorf("default code %q, want %q", c.got, c.want)
		}
		if err := api.ValidateCode(c.got); err != nil {
			t.Errorf("default code %q is not valid: %v", c.got, err)
		}
	}
	for uri, want := range map[string]struct {
		code api.PermissionCode
		kind string
	}{
		"ktw-methods:features/add":       {"features:add", api.CodeKindMethod},
		"ktw-agents:tasks/intake":        {"task:run:intake", api.CodeKindTask},
		"ktw-agents:action-surface/list": {"action-surface:list", api.CodeKindMethod},
		"ktw-wiki:publication/push":      {"publication:push", api.CodeKindMethod},
	} {
		code, kind, err := api.CallableCode(uri)
		if err != nil || code != want.code || kind != want.kind {
			t.Errorf("CallableCode(%q) = %q %q %v, want %q %q", uri, code, kind, err, want.code, want.kind)
		}
	}
	for _, uri := range []string{"ktw-agents:/skill/work-loop", "ktw-agents:!cwm/objects", "no-provider", "ktw-agents:features"} {
		if _, _, err := api.CallableCode(uri); err == nil {
			t.Errorf("CallableCode(%q) resolved a code: a resource, an event or a malformed URI has none", uri)
		}
	}
}

func TestAPermissionCodeIsTwoOrMoreLowercaseSegments(t *testing.T) {
	for _, ok := range []api.PermissionCode{"release:approve", "task:run:confluence-import", "fs:write", "release:*", "gate:ready-to-release:confirm", "path:modules/x.md"} {
		if err := api.ValidateCode(ok); err != nil {
			t.Errorf("%q refused: %v", ok, err)
		}
	}
	for _, bad := range []api.PermissionCode{"", "release", "Release:approve", "release:", ":approve", "release approve:x"} {
		if err := api.ValidateCode(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

// Decision A2: the subject is the principal - the person, or the agent and the
// person it acts for.
func TestEveryCallCarriesItsPrincipal(t *testing.T) {
	ctx := context.Background()
	if p := api.PrincipalOf(ctx, "jdoe"); p != (api.Principal{Kind: "person", ID: "jdoe"}) {
		t.Errorf("no cause: %+v, want the universe's person", p)
	}
	pc := cause.WithCause(ctx, cause.Cause{Kind: cause.KindPerson, ID: "alice"})
	if p := api.PrincipalOf(pc, "jdoe"); p != (api.Principal{Kind: "person", ID: "alice"}) {
		t.Errorf("a person's cause: %+v, want alice", p)
	}
	ac := cause.WithCause(ctx, cause.Cause{Kind: cause.KindAgent, ID: "run-7"})
	if p := api.PrincipalOf(ac, "jdoe"); p != (api.Principal{Kind: "agent", ID: "run-7", For: "jdoe"}) {
		t.Errorf("an agent: %+v, want run-7 acting for jdoe", p)
	}
	rc := cause.WithCause(ctx, cause.Cause{Kind: cause.KindAgent, ID: "claude-code", Relay: &cause.Relay{Person: "bob", Confirmed: cause.ConfirmedPasskey}})
	if p := api.PrincipalOf(rc, "jdoe"); p.For != "bob" {
		t.Errorf("an agent relaying bob's act: %+v, want acting for bob", p)
	}
}

type fixed struct {
	name string
	reqs []api.Requirement
	err  error
}

func (f fixed) Name() string { return f.name }
func (f fixed) Requirements(context.Context) ([]api.Requirement, error) {
	return f.reqs, f.err
}

func TestCodesListsEverySourceSortedOncePerActAndFiltersByKind(t *testing.T) {
	callables := Callables{List: func(context.Context) ([]string, error) {
		return []string{"ktw-agents:todos/pull", "ktw-agents:tasks/intake", "ktw-agents:/skill/x", "ktw-agents:todos/pull"}, nil
	}}
	kinds := fixed{name: "ask-kinds", reqs: []api.Requirement{
		{Code: api.HumanTaskCode("release", "approve"), Kind: api.CodeKindHumanTask, Of: "approve"},
		{Code: "release:approve-notes", Kind: api.CodeKindHumanTask, Of: "approve-notes", Declared: true},
	}}
	tools := fixed{name: "tools", reqs: []api.Requirement{{Code: api.ToolCode("file-write"), Kind: api.CodeKindTool, Of: "file-write"}}}
	s := Service{Sources: func(context.Context) []api.PermissionSource { return []api.PermissionSource{callables, kinds, tools} }}

	got, err := s.Codes(context.Background(), api.PermissionCodesInput{})
	if err != nil {
		t.Fatal(err)
	}
	var codes []api.PermissionCode
	for _, r := range got.Requirements {
		codes = append(codes, r.Code)
		if r.Source == "" {
			t.Errorf("%s has no source", r.Code)
		}
	}
	want := []api.PermissionCode{"release:approve", "release:approve-notes", "task:run:intake", "todos:pull", "tool:file-write"}
	if len(codes) != len(want) {
		t.Fatalf("codes %v, want %v", codes, want)
	}
	for i := range want {
		if codes[i] != want[i] {
			t.Fatalf("codes %v, want %v", codes, want)
		}
	}

	only, err := s.Codes(context.Background(), api.PermissionCodesInput{Kind: api.CodeKindHumanTask})
	if err != nil || len(only.Requirements) != 2 {
		t.Fatalf("kind filter: %+v %v, want the two human-task codes", only.Requirements, err)
	}
}

func TestCodesRefusesASourceThatDeclaresAMalformedCode(t *testing.T) {
	bad := fixed{name: "bad", reqs: []api.Requirement{{Code: "Release", Kind: api.CodeKindHumanTask, Of: "approve"}}}
	s := Service{Sources: func(context.Context) []api.PermissionSource { return []api.PermissionSource{bad} }}
	if _, err := s.Codes(context.Background(), api.PermissionCodesInput{}); err == nil {
		t.Fatal("a malformed code was listed")
	}
	failing := fixed{name: "down", err: errors.New("boom")}
	s = Service{Sources: func(context.Context) []api.PermissionSource { return []api.PermissionSource{failing} }}
	if _, err := s.Codes(context.Background(), api.PermissionCodesInput{}); err == nil {
		t.Fatal("a failing source was hidden")
	}
}

// Decision A4: declaration only - until grants exist, the check allows, and
// says for whom.
func TestCheckAllowsUntilGrantsExistAndNamesThePrincipal(t *testing.T) {
	s := Service{Person: func() string { return "jdoe" }}
	ctx := cause.WithCause(context.Background(), cause.Cause{Kind: cause.KindAgent, ID: "run-9"})
	got, err := s.Check(ctx, api.PermissionCheckInput{Code: "release:approve", Scope: "roadmap.RL-033"})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Allowed || got.Principal != (api.Principal{Kind: "agent", ID: "run-9", For: "jdoe"}) {
		t.Errorf("check: %+v, want allowed for run-9 acting for jdoe", got)
	}
	if _, err := s.Check(ctx, api.PermissionCheckInput{Code: "approve"}); err == nil {
		t.Error("a malformed code was checked")
	}
}
