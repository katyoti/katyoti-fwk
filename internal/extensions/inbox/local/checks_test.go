// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	guideapi "github.com/katyoti/katyoti-fwk/internal/extensions/guide/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	inboxcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/contrib"
	inboxlocal "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/local"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// The guide's inbox check answers what the welcome tour asks at its inbox
// step: not while the request waits, yes once answered, never for a dismissed
// one, and only for the named request when `with.title` is given.
func TestInboxChecks_RequestAnswered(t *testing.T) {
	ctx, cwm := cwmapp.New(t, inboxlocal.Extension(), inboxlocal.ChecksExtension(), inboxcontrib.Pack(""), cwmapp.SystemModules(t))
	seedInbox(t, ctx, cwm)
	svc, err := application.UseService[api.InboxService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	checks, err := application.UseService[api.InboxChecks](ctx)
	if err != nil {
		t.Fatal(err)
	}
	ask := func(with map[string]string) guideapi.CheckResult {
		t.Helper()
		r, err := checks.RequestAnswered(ctx, guideapi.CheckInput{Universe: "u", With: with})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}

	if r := ask(nil); r.OK {
		t.Fatalf("empty inbox: %+v", r)
	}
	welcome := file(t, ctx, svc, "Welcome to KaTyoti")
	other := file(t, ctx, svc, "Something else")
	if r := ask(map[string]string{"title": "Welcome to KaTyoti"}); r.OK {
		t.Fatalf("pending request passed: %+v", r)
	}
	if _, err := svc.Dismiss(ctx, api.DismissInput{ID: other.ID}); err != nil {
		t.Fatal(err)
	}
	if r := ask(map[string]string{"title": "Something else"}); r.OK {
		t.Fatalf("a dismissed request is not an answer: %+v", r)
	}
	if _, err := svc.Answer(ctx, api.AnswerInput{ID: welcome.ID, Note: "yes"}); err != nil {
		t.Fatal(err)
	}
	if r := ask(map[string]string{"title": "Welcome to KaTyoti"}); !r.OK || r.Detail == "" {
		t.Fatalf("answered request: %+v", r)
	}
	if r := ask(nil); !r.OK {
		t.Fatalf("any answered request: %+v", r)
	}
	if r := ask(map[string]string{"title": "Never filed"}); r.OK {
		t.Fatalf("unknown title passed: %+v", r)
	}
}
