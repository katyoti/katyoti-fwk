// SPDX-License-Identifier: Apache-2.0

package core

import (
	"encoding/json"
	"os"
	"os/exec"
	"testing"
	"time"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/journal/api"
	journalcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/journal/contrib"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

func written(t *testing.T, action, id, module string, at time.Time) cwmapi.EventEnvelope {
	t.Helper()
	p, _ := json.Marshal(cwmapi.ObjectEventPayload{ObjectID: id, Module: module, Type: "note", Action: action})
	subject := cwmapi.ObjectUpdatedSubject
	if action == api.ActionCreated {
		subject = cwmapi.ObjectCreatedSubject
	}
	return cwmapi.EventEnvelope{Topic: subject("t", "u", "w", cwmapi.SubjectToken(id)), Timestamp: at, Payload: p}
}

func movedEv(t *testing.T, from, to, module string, at time.Time) cwmapi.EventEnvelope {
	t.Helper()
	p, _ := json.Marshal(cwmapi.ObjectMovedEventPayload{ObjectID: to, Module: module, FromID: from, FromModule: "billing"})
	return cwmapi.EventEnvelope{Topic: cwmapi.ObjectMovedSubject("t", "u", "w", cwmapi.SubjectToken(to)), Timestamp: at, Payload: p}
}

// TestTimesAreTheJournals: an object's created time is its created record,
// its changed time its newest record — through a move, under its new id.
func TestTimesAreTheJournals(t *testing.T) {
	ctx, _ := cwmapp.New(t, journalcontrib.Pack(""), cwmapp.SystemModules(t))
	s := New(90)
	t0 := time.Now().UTC().Add(-3 * time.Hour).Truncate(time.Second)
	t1, t2 := t0.Add(time.Hour), t0.Add(2*time.Hour)
	for _, ev := range []cwmapi.EventEnvelope{
		written(t, api.ActionCreated, "billing.x", "billing", t0),
		written(t, api.ActionUpdated, "billing.x", "billing", t1),
		written(t, api.ActionCreated, "billing.other", "billing", t1),
		movedEv(t, "billing.x", "payments.x", "payments", t2),
	} {
		if err := s.Record(ctx, ev); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.Times(ctx, api.TimesInput{ID: "payments.x"})
	if err != nil {
		t.Fatal(err)
	}
	if !got.CreatedAt.Equal(t0) || !got.UpdatedAt.Equal(t2) {
		t.Fatalf("times = %+v, want created %s changed %s", got, t0, t2)
	}
	if _, err := s.Times(ctx, api.TimesInput{}); err == nil {
		t.Error("an empty id must be refused")
	}
}

// TestTimesFallBackOnGit: what the journal does not answer is read from git's
// log of the object's file — and a git creation later than the journal's
// oldest record of the object gives way to that record.
func TestTimesFallBackOnGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	root := t.TempDir()
	ctx, cwm := cwmapp.NewAt(t, root, journalcontrib.Pack(""), cwmapp.SystemModules(t))
	cwmapp.Module(t, ctx, cwm, "billing")
	res, err := cwm.ObjectResolve(ctx, cwmapi.ObjectResolveInput{Ref: "billing"})
	if err != nil || res.Path == "" || res.ProjectRoot == "" {
		t.Fatalf("resolve billing: %+v %v", res, err)
	}
	git := func(date string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", res.ProjectRoot, "-c", "user.name=ana", "-c", "user.email=a@x", "-c", "commit.gpgsign=false"}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_DATE="+date, "GIT_COMMITTER_DATE="+date)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("", "init", "-q")
	git("2026-01-02T10:00:00Z", "add", "-A")
	git("2026-01-02T10:00:00Z", "commit", "-q", "-m", "first")
	if err := os.WriteFile(res.Path, append(mustRead(t, res.Path), []byte("description: changed\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	git("2026-02-03T10:00:00Z", "commit", "-q", "-am", "second")

	s := New(90)
	got, err := s.Times(ctx, api.TimesInput{ID: "billing"})
	if err != nil {
		t.Fatal(err)
	}
	first, last := time.Date(2026, 1, 2, 10, 0, 0, 0, time.UTC), time.Date(2026, 2, 3, 10, 0, 0, 0, time.UTC)
	if !got.CreatedAt.Equal(first) || !got.UpdatedAt.Equal(last) {
		t.Fatalf("git times = %+v, want created %s changed %s", got, first, last)
	}

	// A journal record, no created one: the change is the journal's, the
	// creation still git's.
	now := time.Now().UTC().Truncate(time.Second)
	if err := s.Record(ctx, written(t, api.ActionUpdated, "billing", "billing", now)); err != nil {
		t.Fatal(err)
	}
	got, _ = s.Times(ctx, api.TimesInput{ID: "billing"})
	if !got.CreatedAt.Equal(first) || !got.UpdatedAt.Equal(now) {
		t.Fatalf("mixed times = %+v, want created %s changed %s", got, first, now)
	}
}

// TestWithoutGitTheOldestRecordIsTheCreation: the object was there at the
// journal's oldest record, so a later first commit is not its creation.
func TestWithoutGitTheOldestRecordIsTheCreation(t *testing.T) {
	ctx, _ := cwmapp.New(t, journalcontrib.Pack(""), cwmapp.SystemModules(t))
	s := New(90)
	at := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	if err := s.Record(ctx, written(t, api.ActionUpdated, "billing.y", "billing", at)); err != nil {
		t.Fatal(err)
	}
	// billing.y has no file: git answers nothing, so the oldest record stands.
	got, err := s.Times(ctx, api.TimesInput{ID: "billing.y"})
	if err != nil {
		t.Fatal(err)
	}
	if !got.CreatedAt.Equal(at) || !got.UpdatedAt.Equal(at) {
		t.Fatalf("times = %+v, want both %s", got, at)
	}
}

func mustRead(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
