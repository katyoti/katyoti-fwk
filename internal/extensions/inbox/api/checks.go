// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
	guideapi "github.com/katyoti/katyoti-fwk/internal/extensions/guide/api"
)

// InboxChecks answers the guide's checks about the inbox: read-only questions
// a recipe asks at its last step (modules/guide/concepts.md, decision OD-009).
// Its own slot (`inbox-checks`), beside the InboxService it reads.
type InboxChecks interface {
	// RequestAnswered: an approval request in the inbox has been answered —
	// the one titled `with.title` when given, else any.
	RequestAnswered(ctx context.Context, in guideapi.CheckInput) (guideapi.CheckResult, error)
}

// InboxChecksServiceDef declares the checks for KTSP publication. Every
// method is read-only: it may serve as a recipe's check and leaves no record.
var InboxChecksServiceDef = meta.ServiceDef{
	Name:    "inbox-checks",
	Short:   "Read-only checks a guide recipe runs to prove its result: a request answered",
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		"RequestAnswered": {Short: "Has an approval request (with.title, else any) been answered?", ReadOnly: true},
	},
}
