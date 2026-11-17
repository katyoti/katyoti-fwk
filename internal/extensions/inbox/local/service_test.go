// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	schemaparser "github.com/katyoti/katyoti-fwk/internal/core/schema/parser"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	inboxcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/contrib"
	inboxlocal "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/local"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
	"github.com/santhosh-tekuri/jsonschema/v5"
)

func setup(t *testing.T) (context.Context, cwmapi.CWMService, api.InboxService) {
	t.Helper()
	ctx, cwm := cwmapp.New(t, inboxlocal.Extension(), inboxcontrib.Pack(""), cwmapp.SystemModules(t))
	seedInbox(t, ctx, cwm)
	svc, err := application.UseService[api.InboxService](ctx)
	if err != nil {
		t.Fatalf("resolve InboxService: %v", err)
	}
	return ctx, cwm, svc
}

// seedInbox writes what the inbox's bootstrap writes: the sys-inbox module and
// its inbox. The test app declares no project object, so the bootstrap itself
// stays silent.
func seedInbox(t *testing.T, ctx context.Context, cwm cwmapi.CWMService) {
	t.Helper()
	mod := cwmapi.Object{ID: "sys-inbox", Type: cwmapi.SystemModuleType}
	mod.Name = "sys-inbox"
	inbox := cwmapi.Object{ID: "sys-inbox.inbox", Type: "inbox"}
	inbox.Name = "Inbox"
	inbox.Parent = "sys-inbox"
	for _, o := range []cwmapi.Object{mod, inbox} {
		if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "sys-inbox", Object: o}); err != nil {
			t.Fatalf("seed %s: %v", o.ID, err)
		}
	}
}

func file(t *testing.T, ctx context.Context, svc api.InboxService, title string) api.Entry {
	t.Helper()
	out, err := svc.File(ctx, api.FileInput{Title: title, Description: "why " + title, RequestedBy: "task-1", BlocksTask: true})
	if err != nil {
		t.Fatalf("File %q: %v", title, err)
	}
	return out.Request
}

// entryStatus is what the service reports: pending, dismissed, or the decision.
func entryStatus(t *testing.T, ctx context.Context, svc api.InboxService, id string) string {
	t.Helper()
	g, err := svc.Get(ctx, api.GetInput{ID: id})
	if err != nil {
		t.Fatalf("Get %s: %v", id, err)
	}
	return g.Entry.Status
}

// state is the request's lifecycle state (its workflow, approval-request).
func state(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, id string) string {
	t.Helper()
	m, _ := cwmapi.ModuleOfObjectID(id)
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: m, ID: id})
	if err != nil {
		t.Fatalf("ObjectGet %s: %v", id, err)
	}
	life, _ := got.Object.Lifecycle()
	return life.State
}

func read(t *testing.T, ctx context.Context, cwm cwmapi.CWMService, id string) (cwmapi.Object, map[string]any) {
	t.Helper()
	m, _ := cwmapi.ModuleOfObjectID(id)
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: m, ID: id})
	if err != nil {
		t.Fatalf("ObjectGet %s: %v", id, err)
	}
	c := got.Object
	body := map[string]any{}
	_ = json.Unmarshal(c.FieldsJSON(), &body)
	return c, body
}

func triageIDs(t *testing.T, ctx context.Context, svc api.InboxService) []string {
	t.Helper()
	out, err := svc.Triage(ctx, api.TriageInput{})
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, len(out.Entries))
	for i, e := range out.Entries {
		ids[i] = e.ID
	}
	return ids
}

// TestInboxService drives every verb against a real CWM service and reads each
// effect back through the Object API.
func TestInboxService(t *testing.T) {
	ctx, cwm, svc := setup(t)

	// File — a pending approval-request under the sys-inbox inbox.
	r1 := file(t, ctx, svc, "Deploy?")
	if m, _ := cwmapi.ModuleOfObjectID(r1.ID); m != "sys-inbox" || r1.Inbox != "sys-inbox.inbox" {
		t.Fatalf("filed where? id %s inbox %s", r1.ID, r1.Inbox)
	}
	_, body := read(t, ctx, cwm, r1.ID)
	if body["requested_by"] != "task-1" || body["blocks_task"] != true || body["requested_at"] == nil || body["status"] != nil {
		t.Fatalf("stored request: body %v", body)
	}

	// Get — the request as the service reads it.
	g, err := svc.Get(ctx, api.GetInput{ID: r1.ID})
	if err != nil || g.Entry.Description != "why Deploy?" || g.Entry.Status != "pending" {
		t.Fatalf("Get: %v %+v", err, g.Entry)
	}

	// A non-request entry filed for triage (an idea — a feature in state idea, say) sits in the inbox too.
	idea := cwmapi.NewRecordObject("sys-inbox", "idea-1", "feature", "sys-inbox.inbox", "An idea", nil)
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "sys-inbox", Object: idea}); err != nil {
		t.Fatal(err)
	}
	r2 := file(t, ctx, svc, "Merge?")

	// Triage — both requests and the idea.
	if ids := triageIDs(t, ctx, svc); len(ids) != 3 {
		t.Fatalf("Triage: %v", ids)
	}

	// Answer — status and note written; what the waiting task reads.
	if _, err := svc.Answer(ctx, api.AnswerInput{ID: r1.ID, Note: "go ahead"}); err != nil {
		t.Fatal(err)
	}
	_, body = read(t, ctx, cwm, r1.ID)
	if st := entryStatus(t, ctx, svc, r1.ID); st != "approved" || state(t, ctx, cwm, r1.ID) != "answered" ||
		body["decision_note"] != "go ahead" || body["resolved_by"] != "user" || body["description"] != "why Deploy?" {
		t.Fatalf("answered request: status %q body %v", st, body)
	}

	// Dismiss — a request is closed and kept; the idea is removed.
	if out, err := svc.Dismiss(ctx, api.DismissInput{ID: r2.ID}); err != nil || out.Removed {
		t.Fatalf("Dismiss request: %v %+v", err, out)
	}
	if st := entryStatus(t, ctx, svc, r2.ID); st != "dismissed" || state(t, ctx, cwm, r2.ID) != "dismissed" {
		t.Fatalf("dismissed request status = %q", st)
	}
	if out, err := svc.Dismiss(ctx, api.DismissInput{ID: idea.ID}); err != nil || !out.Removed {
		t.Fatalf("Dismiss idea: %v %+v", err, out)
	}
	if _, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "sys-inbox", ID: idea.ID}); !errors.Is(err, cwmapi.ErrNotFound) {
		t.Fatalf("the dismissed idea is still there: %v", err)
	}

	// Triage — nothing left: answered and dismissed requests leave the list.
	if ids := triageIDs(t, ctx, svc); len(ids) != 0 {
		t.Fatalf("Triage at the end: %v", ids)
	}
}

// An entry is answered once: a second answer changes nothing and says so -
// refused (ok false) with who answered it, when and what - so the card a
// person clicks again shows it instead of silence. A dismissed one the same.
func TestInboxAnswerOnlyOnce(t *testing.T) {
	ctx, cwm, svc := setup(t)
	r := file(t, ctx, svc, "Q")
	if _, err := svc.Answer(ctx, api.AnswerInput{ID: r.ID, Decision: "rejected", Note: "not yet", By: "jdoe"}); err != nil {
		t.Fatal(err)
	}
	again, err := svc.Answer(ctx, api.AnswerInput{ID: r.ID, Decision: "approved"})
	if err != nil || again.OK == nil || *again.OK {
		t.Fatalf("a second answer: %+v, %v; want refused (ok false), no error", again, err)
	}
	for _, want := range []string{"Already rejected", "jdoe", "not yet"} {
		if !strings.Contains(again.Message, want) {
			t.Errorf("the second answer says %q; want it to name %q", again.Message, want)
		}
	}
	d := file(t, ctx, svc, "Q2")
	if _, err := svc.Dismiss(ctx, api.DismissInput{ID: d.ID}); err != nil {
		t.Fatal(err)
	}
	if out, err := svc.Answer(ctx, api.AnswerInput{ID: d.ID, Decision: "approved"}); err != nil || out.OK == nil || *out.OK || !strings.HasPrefix(out.Message, "Already dismissed") {
		t.Errorf("answering a dismissed entry: %+v, %v; want already dismissed", out, err)
	}
	if st := entryStatus(t, ctx, svc, r.ID); st != "rejected" || state(t, ctx, cwm, r.ID) != "answered" {
		t.Fatalf("the first answer was overwritten: %q", st)
	}
}

func TestInboxPendingIsNotAnAnswer(t *testing.T) {
	ctx, _, svc := setup(t)
	r := file(t, ctx, svc, "Q")
	if _, err := svc.Answer(ctx, api.AnswerInput{ID: r.ID, Decision: "pending"}); !errors.Is(err, cwmapi.ErrInvalidInput) {
		t.Fatalf("answering pending: err = %v", err)
	}
}

func TestInboxAnswerOnlyARequest(t *testing.T) {
	ctx, cwm, svc := setup(t)
	idea := cwmapi.NewRecordObject("sys-inbox", "idea-1", "feature", "sys-inbox.inbox", "An idea", nil)
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "sys-inbox", Object: idea}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Answer(ctx, api.AnswerInput{ID: idea.ID}); !errors.Is(err, cwmapi.ErrInvalidInput) {
		t.Fatalf("answering an idea: err = %v", err)
	}
}

func TestInboxTouchesOnlyItsEntries(t *testing.T) {
	ctx, cwm, svc := setup(t)
	cwmapp.Module(t, ctx, cwm, "m")
	g := cwmapi.NewContainerObject("m", "G-001", "goal", "m", "A goal", "")
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "m", Object: g}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Dismiss(ctx, api.DismissInput{ID: g.ID}); !errors.Is(err, cwmapi.ErrInvalidInput) {
		t.Fatalf("dismissing a goal outside the inbox: err = %v", err)
	}
	if _, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: "m", ID: g.ID}); err != nil {
		t.Fatalf("the goal was deleted: %v", err)
	}
}

func TestInboxUpdatedEventURIUsesTheSubjectToken(t *testing.T) {
	id := "sys-inbox.0192-abc"
	if got, want := api.UpdatedEventURI(id), "cwm:!workspace/object/"+cwmapi.SubjectToken(id)+"/updated"; got != want {
		t.Fatalf("UpdatedEventURI = %q, want %q", got, want)
	}
}

// TestEnsureGivesAProjectItsInboxOnce: a project with no inbox gets sys-inbox and
// its inbox; a second Ensure creates nothing.
func TestEnsureGivesAProjectItsInboxOnce(t *testing.T) {
	ctx, cwm := cwmapp.New(t, inboxlocal.Extension(), inboxcontrib.Pack(""), cwmapp.SystemModules(t))
	svc, err := application.UseService[api.InboxService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	first, err := svc.Ensure(ctx, api.EnsureInput{})
	if err != nil || first.Inbox != "sys-inbox.inbox" || !first.Created {
		t.Fatalf("first Ensure = %+v (%v), want sys-inbox.inbox created", first, err)
	}
	again, err := svc.Ensure(ctx, api.EnsureInput{})
	if err != nil || again.Inbox != "sys-inbox.inbox" || again.Created {
		t.Fatalf("second Ensure = %+v (%v), want the same inbox, not created", again, err)
	}
	list, _ := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Type: api.InboxType})
	if len(list.Objects) != 1 {
		t.Fatalf("inboxes after two Ensures: %d, want 1", len(list.Objects))
	}
}

// TestEnsureKeepsTheInboxAProjectHas: a project that already has an inbox keeps it.
func TestEnsureKeepsTheInboxAProjectHas(t *testing.T) {
	ctx, _, svc := setup(t)
	out, err := svc.Ensure(ctx, api.EnsureInput{})
	if err != nil || out.Created || out.Inbox != "sys-inbox.inbox" {
		t.Fatalf("Ensure on a seeded project = %+v (%v)", out, err)
	}
}

// TestFilingNeedsNoBootstrapFirst: the inbox is created the moment a request
// needs it — the bootstrap hook makes it appear early, it is not a precondition.
func TestFilingNeedsNoBootstrapFirst(t *testing.T) {
	ctx, _ := cwmapp.New(t, inboxlocal.Extension(), inboxcontrib.Pack(""), cwmapp.SystemModules(t))
	svc, err := application.UseService[api.InboxService](ctx)
	if err != nil {
		t.Fatal(err)
	}
	out, err := svc.File(ctx, api.FileInput{Title: "q", Description: "why", RequestedBy: "task-1"})
	if err != nil || out.Request.ID == "" {
		t.Fatalf("File on a project with no inbox = %+v (%v)", out, err)
	}
}

// TestEnsureKeepsAMigratedInboxElsewhere: a project whose inbox lives in another
// module keeps it — Ensure creates no sys-inbox beside it.
func TestEnsureKeepsAMigratedInboxElsewhere(t *testing.T) {
	ctx, cwm := cwmapp.New(t, inboxlocal.Extension(), inboxcontrib.Pack(""), cwmapp.SystemModules(t))
	live := cwmapi.Object{ID: "live.inbox", Type: api.InboxType}
	live.Name = "Inbox"
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "live", Object: live}); err != nil {
		t.Fatal(err)
	}
	svc, _ := application.UseService[api.InboxService](ctx)
	out, err := svc.Ensure(ctx, api.EnsureInput{})
	if err != nil || out.Inbox != "live.inbox" || out.Created {
		t.Fatalf("Ensure = %+v (%v), want the migrated live.inbox kept", out, err)
	}
	if list, _ := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Type: api.InboxType}); len(list.Objects) != 1 {
		t.Fatalf("inboxes = %d, want the one there was", len(list.Objects))
	}
}

// TestRelocateMovesAMigratedInboxIntoSysInbox: an inbox kept in another module
// is moved, with its entries, into sys-inbox — only with apply; the requests
// stay answerable where they land; a second Relocate has nothing to do.
func TestRelocateMovesAMigratedInboxIntoSysInbox(t *testing.T) {
	ctx, cwm := cwmapp.New(t, inboxlocal.Extension(), inboxcontrib.Pack(""), cwmapp.SystemModules(t))
	live := cwmapi.Object{ID: "live.inbox-1", Type: api.InboxType}
	live.Name = "Inbox"
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "live", Object: live}); err != nil {
		t.Fatal(err)
	}
	svc, _ := application.UseService[api.InboxService](ctx)
	req := file(t, ctx, svc, "keep me")
	if m, _ := cwmapi.ModuleOfObjectID(req.ID); m != "live" {
		t.Fatalf("a request filed before the move lands in %s, want live", m)
	}

	plan, err := svc.Relocate(ctx, api.RelocateInput{})
	if err != nil || plan.Applied || len(plan.Moves) != 2 || plan.Moves[0].From != "live.inbox-1" || plan.Inbox != "sys-inbox.inbox-1" {
		t.Fatalf("dry-run = %+v (%v)", plan, err)
	}
	if list, _ := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Module: "sys-inbox"}); len(list.Objects) != 0 {
		t.Fatalf("a dry run wrote %d object(s) in sys-inbox", len(list.Objects))
	}

	done, err := svc.Relocate(ctx, api.RelocateInput{Apply: true})
	if err != nil || !done.Applied || done.Inbox != "sys-inbox.inbox-1" {
		t.Fatalf("apply = %+v (%v)", done, err)
	}
	if list, _ := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Module: "live"}); len(list.Objects) != 0 {
		t.Fatalf("%d object(s) left in live after the move", len(list.Objects))
	}
	moved := done.Moves[1].To
	if m, _ := cwmapi.ModuleOfObjectID(moved); m != "sys-inbox" {
		t.Fatalf("the request moved to %s", moved)
	}
	if _, err := svc.Answer(ctx, api.AnswerInput{ID: moved}); err != nil {
		t.Fatalf("answer the moved request: %v", err)
	}
	again, err := svc.Relocate(ctx, api.RelocateInput{Apply: true})
	if err != nil || len(again.Moves) != 0 || again.Inbox != "sys-inbox.inbox-1" {
		t.Fatalf("second relocate = %+v (%v), want nothing to do", again, err)
	}
}

// An entry names the panel of what it is about the Inbox opens it on — where
// it is answered (a person step's « À faire » card): filed with one, it is
// read back with it; filed without, it has none (its object's first panel).
func TestEntryOpensOnItsPanel(t *testing.T) {
	ctx, _, svc := setup(t)
	on, err := svc.File(ctx, api.FileInput{Title: "Valider — Print invoices", About: "billing.RQ-001", Panel: "person-queue", Expects: api.ExpectsVerdict})
	if err != nil {
		t.Fatal(err)
	}
	got, err := svc.Get(ctx, api.GetInput{ID: on.Request.ID})
	if err != nil {
		t.Fatal(err)
	}
	if got.Entry.Panel != "person-queue" || got.Entry.About != "billing.RQ-001" {
		t.Errorf("the entry = %+v; want about billing.RQ-001, opened on person-queue", got.Entry)
	}
	plain := file(t, ctx, svc, "May I deploy?")
	if plain.Panel != "" {
		t.Errorf("an entry filed with no panel opens on %q", plain.Panel)
	}
}

// TestANoticeIsFiled: a notice - what a demand became (planification), read
// and dismissed - is filed through the service, and what it stores holds to the
// shipped approval-request data type, whose expects names notice: CWM checks a
// stored object against that schema in production.
func TestANoticeIsFiled(t *testing.T) {
	ctx, cwm, svc := setup(t)
	out, err := svc.File(ctx, api.FileInput{Title: "The demand became billing.RQ-001", RequestedBy: "planification:billing.RQ-001", About: "billing.RQ-001", Expects: api.ExpectsNotice})
	if err != nil {
		t.Fatalf("filing a notice: %v", err)
	}
	_, body := read(t, ctx, cwm, out.Request.ID)
	if body["expects"] != api.ExpectsNotice {
		t.Fatalf("stored expects = %v; want notice", body["expects"])
	}
	compiled, err := schemaparser.CompileJSONSchema(ctx,
		cwmapp.ShippedFile(t, "app/ktw-agents/embed/cwm/data-types/approval-request.dsl.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource("approval-request", bytes.NewReader(compiled)); err != nil {
		t.Fatal(err)
	}
	sch, err := c.Compile("approval-request")
	if err != nil {
		t.Fatal(err)
	}
	if err := sch.Validate(body); err != nil {
		t.Errorf("the shipped approval-request data type refuses a filed notice: %v", err)
	}
	if _, err := svc.Dismiss(ctx, api.DismissInput{ID: out.Request.ID}); err != nil {
		t.Fatalf("dismissing a notice: %v", err)
	}
	if s := entryStatus(t, ctx, svc, out.Request.ID); s != api.StatusDismissed {
		t.Errorf("a dismissed notice is %q; want dismissed", s)
	}
}
