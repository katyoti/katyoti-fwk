// SPDX-License-Identifier: Apache-2.0

package runtime

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	inboxapi "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	toolsapi "github.com/katyoti/katyoti-fwk/internal/extensions/tools/api"
)

// permissionRequestKey is the key, in a saved batch's Resume, of the inbox
// request a tool's permission waits on. A batch carrying it resumes by reading
// the person's answer — the tool itself never sees it.
const permissionRequestKey = "permission_request"

// permissionRecheckEvery is how often a waiting permission reads its request
// again, event or not: an answer whose event no process heard, a request gone.
const permissionRecheckEvery = time.Hour

// permissionDescription is the request's text: the tool's question, then what
// the call would do (its input's dimensions), then how to answer.
func permissionDescription(name string, pres toolsapi.PermissionResult, dims map[string]string) string {
	var b strings.Builder
	question := pres.Message
	if question == "" {
		question = fmt.Sprintf("Allow tool %q to run?", name)
	}
	b.WriteString(question)
	if len(dims) > 0 {
		keys := make([]string, 0, len(dims))
		for k := range dims {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		b.WriteString("\n\nWhat the call would do:\n")
		for _, k := range keys {
			fmt.Fprintf(&b, "\n- **%s**: `%s`", k, dims[k])
		}
	}
	b.WriteString("\n\nApprove to allow this call once; reject to deny it.")
	return b.String()
}

// permissionAwait is the wait of a call on its permission request.
func permissionAwait(requestID string) *agentapi.ErrAwait {
	return &agentapi.ErrAwait{
		EventURI:     inboxapi.UpdatedEventURI(requestID),
		Resume:       map[string]any{permissionRequestKey: requestID},
		Reason:       "a permission in the inbox (" + requestID + ")",
		RecheckEvery: permissionRecheckEvery,
	}
}

// permissionAnswer is where a permission asked through the inbox stands.
type permissionAnswer int

const (
	permissionPending permissionAnswer = iota
	permissionAllowed
	permissionDenied
)

// resumePermission reads the person's answer to a permission request: approved
// → allowed; rejected or dismissed → denied with a message the agent reads (the
// answer's note with it); still pending, or unreadable now → pending (the
// request's next update, or the next recheck, reads it again); gone → denied:
// no answer will come.
func resumePermission(ctx context.Context, name, requestID string) (permissionAnswer, string) {
	inbox, err := application.UseService[inboxapi.InboxService](ctx)
	if err != nil || inbox == nil {
		slog.Warn("permission: no inbox to read the answer from, waiting on", "tool", name, "request", requestID, "error", err)
		return permissionPending, ""
	}
	got, err := inbox.Get(ctx, inboxapi.GetInput{ID: requestID})
	switch {
	case errors.Is(err, inboxapi.ErrGone):
		return permissionDenied, fmt.Sprintf("the permission request %s for %q is gone (deleted or moved out of the inbox): no answer will come and the call is denied — ask again if you still need it", requestID, name)
	case err != nil:
		slog.Warn("permission: reading the request failed, waiting on", "tool", name, "request", requestID, "error", err)
		return permissionPending, ""
	}
	e := got.Entry
	note := ""
	if e.DecisionNote != "" {
		note = ": " + e.DecisionNote
	}
	switch e.Status {
	case "", inboxapi.StatusPending:
		return permissionPending, ""
	case inboxapi.StatusApproved:
		return permissionAllowed, ""
	case inboxapi.StatusRejected:
		return permissionDenied, fmt.Sprintf("the person denied %s%s", name, note)
	case inboxapi.StatusDismissed:
		return permissionDenied, fmt.Sprintf("the person denied %s (the request was dismissed)%s", name, note)
	default:
		// An answer this gate does not read as an approval: fail closed.
		return permissionDenied, fmt.Sprintf("the person denied %s (answered %q)%s", name, e.Status, note)
	}
}
