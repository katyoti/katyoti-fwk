// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	inboxcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/contrib"
	inboxlocal "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/local"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// A question is as long as the asker needs: a 20 KB markdown description is
// filed, answered with a long note (written to the body AND to the lifecycle
// history) and, for a second one, dismissed — no size budget refuses any of
// it. The type declares no max_size_bytes: a cap would either refuse the
// question of a task that is blocked on it, or let it in and then refuse the
// answer that grows it.
func TestALongQuestionIsFiledAnsweredAndDismissed(t *testing.T) {
	root := t.TempDir()
	ctx, cwm := cwmapp.NewAt(t, root, inboxlocal.Extension(), inboxcontrib.Pack(""), cwmapp.SystemModules(t))
	seedInbox(t, ctx, cwm)
	svc, err := application.UseService[api.InboxService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	para := "## Context\n\nThe deployment of **service-x** needs a decision: `kubectl rollout` failed twice.\n\n"
	desc := strings.Repeat(para, 20*1024/len(para)+1)
	note := strings.Repeat("Go ahead, but watch the error rate for an hour. ", 80)

	long, err := svc.File(ctx, api.FileInput{Title: "Deploy service-x?", Description: desc, RequestedBy: "task-1", BlocksTask: true})
	if err != nil {
		t.Fatalf("filing a 20 KB question: %v", err)
	}
	if _, err := svc.Answer(ctx, api.AnswerInput{ID: long.Request.ID, Note: note}); err != nil {
		t.Fatalf("answering it with a note: %v", err)
	}
	if st := state(t, ctx, cwm, long.Request.ID); st != "answered" {
		t.Fatalf("answered request is %q", st)
	}
	if st, err := os.Stat(filepath.Join(root, long.Path)); err != nil || st.Size() < 20*1024 {
		t.Fatalf("the answered request's file: %v (%v), want it whole", st, err)
	}

	other, err := svc.File(ctx, api.FileInput{Title: "Merge?", Description: desc, RequestedBy: "task-2"})
	if err != nil {
		t.Fatalf("filing a second long question: %v", err)
	}
	if _, err := svc.Dismiss(ctx, api.DismissInput{ID: other.Request.ID}); err != nil {
		t.Fatalf("dismissing it: %v", err)
	}
	if st := state(t, ctx, cwm, other.Request.ID); st != "dismissed" {
		t.Fatalf("dismissed request is %q", st)
	}
}
