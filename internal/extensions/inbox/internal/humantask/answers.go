// SPDX-License-Identifier: Apache-2.0

package humantask

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	profileapi "github.com/katyoti/katyoti-fwk/internal/extensions/profile/api"
	schedapi "github.com/katyoti/katyoti-fwk/internal/extensions/scheduler/api"
)

// ANSWERS KEPT: before its to-do is archived and its asker
// told, a human task's answer is checked - the answerer holds the question's
// permission code - and stored as an `answer` record under the object it is
// about - on its to-do alone in a universe no project serves (no sys-inbox).
// A failed check or store returns an error: the collect stops there,
// the to-do stays reported and is handed to the inbox again on the
// scheduler's next tick - nothing goes on with an answer not kept.

// Store checks a reported human task's answer and keeps it on what it is
// about; it answers the record's id (the to-do's own when no project serves
// the universe). Storing twice keeps one record.
func Store(ctx context.Context, t schedapi.Todo) (string, error) {
	a, ok := api.AnswerOf(t)
	if !ok {
		return "", fmt.Errorf("%w: %s carries no answer", cwmapi.ErrInvalidInput, t.ID)
	}
	ask, _ := api.AskOf(t)
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return "", fmt.Errorf("inbox: keep the answer of %s: %w", t.ID, err)
	}
	module, parent, aboutType := placeOf(ctx, cwm, ask.About)
	if err := mayAnswer(ctx, ask, aboutType, a); err != nil {
		return "", err
	}
	if module == systemInbox && !served(ctx, cwm) {
		// No project serves the universe: there is no
		// sys-inbox to keep a record in. The answer stays on its to-do,
		// archived with it in the universe's scheduler folder.
		return t.ID, nil
	}
	rec := recordOf(t, ask, a)
	content, err := json.Marshal(rec)
	if err != nil {
		return "", err
	}
	_, local, _ := strings.Cut(t.ID, ".")
	o := cwmapi.NewRecordObject(module, "answer-"+strings.ToLower(local), api.AnswerType, parent, answerTitle(ask, a), content)
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: module, Object: o}); err != nil && !errors.Is(err, cwmapi.ErrAlreadyExists) {
		return "", fmt.Errorf("inbox: keep the answer of %s on %s: %w", t.ID, parent, err)
	}
	return o.ID, nil
}

// served reports a universe a project serves: its sys-inbox exists. One no
// project serves - the ordinary self-contained layout - has none; another
// fault of the CWM counts as served, its create then says what failed.
func served(ctx context.Context, cwm cwmapi.CWMService) bool {
	_, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: systemInbox, ID: systemInbox})
	return !errors.Is(err, cwmapi.ErrNotFound)
}

// placeOf is where an answer is kept: under what it is about, in its
// module; under sys-inbox when it is about nothing, or about an object gone.
func placeOf(ctx context.Context, cwm cwmapi.CWMService, about string) (module, parent, aboutType string) {
	if about != "" {
		if m, err := cwmapi.ModuleOfObjectID(about); err == nil {
			if got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: m, ID: about}); err == nil {
				return m, about, got.Object.Type
			}
		}
	}
	return systemInbox, systemInbox, ""
}

// mayAnswer refuses an answer whose answerer does not hold the question's
// permission code - its own, else <about type>:<kind> - where the iam
// permission service is served; until grants exist it allows (iam, A4).
func mayAnswer(ctx context.Context, ask api.HumanTask, aboutType string, a api.HumanAnswer) error {
	perms, err := application.UseService[profileapi.PermissionService](ctx)
	if err != nil || perms == nil {
		return nil
	}
	code := profileapi.PermissionCode(ask.Requires)
	if code == "" {
		code = profileapi.HumanTaskCode(aboutType, ask.Kind)
	}
	if profileapi.ValidateCode(code) != nil {
		return nil // no kind to derive a code from: nothing to check against
	}
	got, err := perms.Check(ctx, profileapi.PermissionCheckInput{Code: code, Scope: ask.About,
		Principal: &profileapi.Principal{Kind: a.By.Kind, ID: a.By.ID}})
	if err != nil {
		return fmt.Errorf("inbox: check who answered %s: %w", a.Task, err)
	}
	if !got.Allowed {
		return fmt.Errorf("%w: %s %s does not hold %s: %s", cwmapi.ErrInvalidInput, a.By.Kind, a.By.ID, code, got.Reason)
	}
	return nil
}

// recordOf is the answer as it is kept.
func recordOf(t schedapi.Todo, ask api.HumanTask, a api.HumanAnswer) api.AnswerRecord {
	rec := api.AnswerRecord{
		Task: t.ID, Kind: ask.Kind, Question: ask.Title, Satisfies: ask.Satisfies,
		Status: a.Status, Verdict: a.Verdict, Answer: a.Answer, Summary: a.Summary, Details: a.Details, Data: a.More,
		ByID: a.By.ID, ByKind: a.By.Kind, At: a.At, List: a.List, Run: a.Run,
	}
	if a.Relayed != nil {
		rec.Proof, rec.RelayedBy = a.Relayed.Confirmed, a.Relayed.By
	}
	return rec
}

// answerTitle names a kept answer: the question and who answered.
func answerTitle(ask api.HumanTask, a api.HumanAnswer) string {
	q := ask.Title
	if q == "" {
		q = a.Task
	}
	return q + " - answered by " + a.By.ID
}
