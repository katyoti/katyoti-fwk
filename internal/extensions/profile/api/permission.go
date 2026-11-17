// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cause"
	"github.com/katyoti/katyoti-fwk/internal/core/em"
	"github.com/katyoti/katyoti-fwk/internal/core/meta"
)

// PERMISSION CODES: every interaction and act declares the code it
// needs, so that access can be restricted later - a grant or a deny on a code,
// for a principal, within a scope - without changing the interaction. A code
// says what is done, never who does it. Declaration only: until grants exist,
// everything stays allowed to the universe's person (study human-interaction,
// decisions A1, A2, A4).
//
// The defaults are derived, so most declarations are free:
//   - a human task:            <about type>:<kind>           (release:approve)
//   - a method or an action:   <service>:<method>            (features:add)
//   - a task:                  task:run:<name>               (task:run:intake)
//   - an agent tool:           tool:<name>                   (tool:file-write)
// A behaviour that needs another code declares it (`requires`); the listing
// says which codes are declared and which derived.

// PermissionCode names one permission: colon-separated lowercase segments.
type PermissionCode string

// The kinds of acts that carry a permission code.
const (
	CodeKindHumanTask = "human-task"
	CodeKindMethod    = "method"
	CodeKindTask      = "task"
	CodeKindTool      = "tool"
)

// codeSegment is one segment of a code: lowercase letters, digits, dashes,
// dots and slashes, or a wildcard (a grant's pattern, later).
var codeSegment = regexp.MustCompile(`^(\*|[a-z0-9][a-z0-9./-]*)$`)

// ValidateCode refuses a code that is not two or more well-formed segments.
func ValidateCode(c PermissionCode) error {
	parts := strings.Split(string(c), ":")
	if len(parts) < 2 {
		return fmt.Errorf("permission code %q: want at least two colon-separated segments (<what>:<act>)", c)
	}
	for _, p := range parts {
		if !codeSegment.MatchString(p) {
			return fmt.Errorf("permission code %q: segment %q is not lowercase letters, digits, dashes, dots or slashes", c, p)
		}
	}
	return nil
}

// kebab lowers a name and turns spaces, underscores and camel humps into
// dashes, so a code's segment is always kebab.
func kebab(s string) string {
	var b strings.Builder
	for i, r := range strings.TrimSpace(s) {
		switch {
		case r >= 'A' && r <= 'Z':
			if i > 0 {
				prev := s[i-1]
				if prev >= 'a' && prev <= 'z' || prev >= '0' && prev <= '9' {
					b.WriteByte('-')
				}
			}
			b.WriteRune(r + ('a' - 'A'))
		case r == ' ' || r == '_':
			b.WriteByte('-')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// HumanTaskCode is a human task's default code: <about type>:<kind>. A task
// about no object is about the universe (`universe:<kind>`).
func HumanTaskCode(aboutType, kind string) PermissionCode {
	t := kebab(aboutType)
	if t == "" {
		t = "universe"
	}
	return PermissionCode(t + ":" + kebab(kind))
}

// MethodCode is a method's or an action's default code: <service>:<method>.
func MethodCode(service, method string) PermissionCode {
	return PermissionCode(kebab(service) + ":" + kebab(method))
}

// TaskCode is a task's default code: task:run:<name>.
func TaskCode(name string) PermissionCode {
	return PermissionCode("task:run:" + kebab(name))
}

// ToolCode is an agent tool's default code: tool:<name>.
func ToolCode(name string) PermissionCode {
	return PermissionCode("tool:" + kebab(name))
}

// CallableCode is the default code of a callable URI - a method, an action or
// a task, `<provider>:<service>/<method>` (a task is `<provider>:tasks/<name>`).
// The provider is not part of the code: the same service answers the same
// code wherever it is served.
func CallableCode(uri string) (PermissionCode, string, error) {
	_, rest, ok := strings.Cut(uri, ":")
	if !ok || rest == "" || strings.HasPrefix(rest, "/") || strings.HasPrefix(rest, "!") {
		return "", "", fmt.Errorf("permission code: %q is not a callable URI (<provider>:<service>/<method>)", uri)
	}
	service, method, ok := strings.Cut(rest, "/")
	if !ok || service == "" || method == "" {
		return "", "", fmt.Errorf("permission code: %q is not a callable URI (<provider>:<service>/<method>)", uri)
	}
	if service == "tasks" {
		return TaskCode(method), CodeKindTask, nil
	}
	return MethodCode(service, method), CodeKindMethod, nil
}

// Principal is who acts (decision A2): a person, or an agent run with the
// person it acts for - an agent holds no right of its own.
type Principal struct {
	Kind string `json:"kind" desc:"person or agent (or the kind of cause that acts: hook, action, reaction - each acting for its person)"`
	ID   string `json:"id" desc:"The person's handle, or the run's id."`
	For  string `json:"for,omitempty" desc:"The person an agent (or a hook, an action, a reaction) acts for."`
}

// PrincipalOf is the principal a call carries: read from its cause; person is
// the universe's person, whom an agent acts for unless it relays another's
// act. A call with no cause is the person's own.
func PrincipalOf(ctx context.Context, person string) Principal {
	c, ok := cause.CauseFrom(ctx)
	if !ok || c.Kind == "" {
		return Principal{Kind: cause.KindPerson, ID: person}
	}
	if c.Kind == cause.KindPerson {
		id := c.ID
		if id == "" {
			id = person
		}
		return Principal{Kind: cause.KindPerson, ID: id}
	}
	forWhom := person
	if c.Relay != nil && c.Relay.Person != "" {
		forWhom = c.Relay.Person
	}
	return Principal{Kind: c.Kind, ID: c.ID, For: forWhom}
}

// Requirement is the permission code one act needs.
type Requirement struct {
	Code     PermissionCode `json:"code"`
	Kind     string         `json:"kind" desc:"human-task, method, task or tool"`
	Of       string         `json:"of" desc:"What needs it: the URI of a method or a task, a tool's name, a human task's kind."`
	Declared bool           `json:"declared,omitempty" desc:"Declared by its owner; false = the derived default."`
	Source   string         `json:"source,omitempty" desc:"The source that listed it."`
}

// PermissionSource lists the requirements of the acts one owner offers - the
// human-task kinds, the agent tools, a pack's own. Owners contribute one with
// ContributePermissionSource; the listing reads them all.
type PermissionSource interface {
	Name() string
	Requirements(ctx context.Context) ([]Requirement, error)
}

// ContributePermissionSource adds a source to the listing of permission codes,
// in the binary that serves the PermissionService.
func ContributePermissionSource(s PermissionSource) application.Extension {
	return func(init *application.ApplicationInit) {
		init.AddRegisterPoints("contribution", ".iam.permission-sources.point", func(app *application.Application) error {
			err := application.RegisterPoint(app, em.NewListPoint[PermissionSource]())
			var dup em.ErrDuplicatePointID
			if errors.As(err, &dup) {
				return nil // another source of this binary registered it first
			}
			return err
		})
		init.AddContribute("contribution", ".iam.permission-sources."+s.Name(), func(app *application.Application) error {
			return application.Contribute(app, s)
		})
	}
}

// PermissionServiceDef declares the permission codes' surface, served by the
// process serving the universe (ktw-agents:permission/<method>).
var PermissionServiceDef = meta.ServiceDef{
	Name:    "Permission",
	BusName: "permission",
	Short:   "The permission codes every act declares, and the check that reads them",
	Long:    `Every interaction and act declares the permission code it needs - a human task <about type>:<kind>, a method <service>:<method>, a task task:run:<name>, an agent tool tool:<name> - so that access can be restricted later without changing any of them. Codes lists them, declared or derived; Check answers for a code and the principal a call carries (the person, or the agent and the person it acts for): until grants exist, everything is allowed to the universe's person.`,
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		"Codes": {Short: "List the permission codes of every act this universe offers, declared or derived", ReadOnly: true},
		"Check": {Short: "Answer whether the principal a call carries holds a permission code (allowed until grants exist)", ReadOnly: true},
	},
}

// PermissionService lists the permission codes and checks them.
type PermissionService interface {
	Codes(ctx context.Context, in PermissionCodesInput) (PermissionCodesOutput, error)
	Check(ctx context.Context, in PermissionCheckInput) (PermissionCheckOutput, error)
}

// PermissionCodesInput filters the listing.
type PermissionCodesInput struct {
	Kind string `json:"kind,omitempty" cli:"kind" desc:"Only one kind: human-task, method, task or tool."`
}

// PermissionCodesOutput is every requirement, sorted by code.
type PermissionCodesOutput struct {
	Requirements []Requirement `json:"requirements"`
}

// PermissionCheckInput asks for one code, within a scope; the principal is
// the call's own unless given.
type PermissionCheckInput struct {
	Code      PermissionCode `json:"code" cli:"code" desc:"The permission code." required:"true"`
	Scope     string         `json:"scope,omitempty" cli:"scope" desc:"Where it is asked: an object id, a path, a project."`
	Principal *Principal     `json:"principal,omitempty" desc:"Who asks; empty = the principal the call carries."`
}

// PermissionCheckOutput is the answer and what it was asked for.
type PermissionCheckOutput struct {
	Allowed   bool           `json:"allowed"`
	Code      PermissionCode `json:"code"`
	Principal Principal      `json:"principal"`
	Reason    string         `json:"reason"`
}
