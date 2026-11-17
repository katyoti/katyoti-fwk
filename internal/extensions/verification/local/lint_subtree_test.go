// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"context"
	"errors"
	"sort"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/verification/api"
	verificationlocal "github.com/katyoti/katyoti-fwk/internal/extensions/verification/local"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// LINT A SUBTREE, on a real CWM: a feature and the criterion under it both
// carry a lint hook; Lint{id} lints the feature alone, Lint{id, subtree}
// the feature and the criterion, and an input naming both an id and a module,
// or neither, is refused before any hook runs.
func TestLintSubtreeLintsTheObjectAndItsDescendants(t *testing.T) {
	acts := &actions{replies: map[string]string{lintHook: lintWarn}, calls: map[string]int{}}
	hooked := "hooks:\n  lint:\n    - uri: " + lintHook + "\n"
	pack := fstest.MapFS{
		"capabilities/container.yaml": {Data: []byte("name: container\n")},
		"object-types/module.yaml":    {Data: []byte("name: module\ncapabilities: [container]\n")},
		"object-types/feature.yaml":   {Data: []byte("name: feature\ncapabilities: [container]\n" + hooked)},
		"object-types/criterion.yaml": {Data: []byte("name: criterion\ncapabilities: [container]\n" + hooked)},
	}
	ctx, cwm := cwmapp.New(t,
		cwmapp.Pack(pack),
		verificationlocal.Extension(),
		application.DeclareSimpleService[actionsapi.ActionService](func(context.Context) (actionsapi.ActionService, error) { return acts, nil }),
	)
	cwmapp.Module(t, ctx, cwm, "billing")
	for _, o := range []cwmapi.Object{
		{ID: "billing.f1", Type: "feature", Parent: "billing"},
		{ID: "billing.ac1", Type: "criterion", Parent: "billing.f1"},
		{ID: "billing.f2", Type: "feature", Parent: "billing"},
	} {
		o.Name = o.ID
		if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "billing", Object: o}); err != nil {
			t.Fatalf("create %s: %v", o.ID, err)
		}
	}
	v, err := application.UseService[api.VerificationService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	linted := func(out api.LintOutput) string {
		var ids []string
		for _, f := range out.Findings {
			ids = append(ids, f.ObjectID)
		}
		sort.Strings(ids)
		return strings.Join(ids, ",")
	}

	one, err := v.Lint(ctx, api.LintInput{ID: "billing.f1"})
	if err != nil || linted(one) != "billing.f1" || one.Objects != 1 || one.Runs != 1 {
		t.Errorf("one object: %s objects %d runs %d (%v)", linted(one), one.Objects, one.Runs, err)
	}
	sub, err := v.Lint(ctx, api.LintInput{ID: "billing.f1", Subtree: true})
	if err != nil || linted(sub) != "billing.ac1,billing.f1" || sub.Objects != 2 || sub.Runs != 2 {
		t.Errorf("subtree: %s objects %d runs %d (%v) — want the feature and its criterion, not the sibling", linted(sub), sub.Objects, sub.Runs, err)
	}

	before := acts.count(lintHook)
	for name, in := range map[string]api.LintInput{
		"both":             {ID: "billing.f1", Module: "billing"},
		"neither":          {},
		"subtree, module":  {Module: "billing", Subtree: true},
		"subtree, neither": {Subtree: true},
	} {
		if _, err := v.Lint(ctx, in); !errors.Is(err, cwmapi.ErrInvalidInput) {
			t.Errorf("%s: %v, want refused", name, err)
		}
	}
	if n := acts.count(lintHook) - before; n != 0 {
		t.Errorf("a refused lint ran %d hooks", n)
	}
}
