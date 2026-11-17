// SPDX-License-Identifier: Apache-2.0

// Package guidelint is the guide-recipe contract as a lint — the sibling of
// the skills pack's skill-format and the task pack's task-format. It lives
// with the kind it describes, in a package only ktw-compose and test code
// import: nothing extra links into a sidecar's executable; the census lints at
// build, compose on demand.
//
// Two scopes, as for the other kinds' lints:
//   - LintRecipeBytes judges ONE recipe on its own: its fields, its steps,
//     its URIs' grammar. It needs nothing but the bytes.
//   - LintChecks judges what a recipe names in the universe: every `check`
//     must be a method that declares itself read-only, runs synchronously and
//     answers `ok` and `detail` (modules/guide/decisions/OD-009). It needs a
//     way to describe an action, so it runs where the universe is known.
package guidelint

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	guideapi "github.com/katyoti/katyoti-fwk/internal/extensions/guide/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/methods/verify"
)

// reIdentity is a recipe identity as `requires` names it: a file name
// without extension, kebab-case.
var reIdentity = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// knownFields are the recipe's top-level fields — read from guideapi.Recipe's
// yaml tags, so a field added to the type is known here at once; anything else
// is a typo the parser would silently drop.
var knownFields = yamlFields(reflect.TypeOf(guideapi.Recipe{}))

var knownStepFields = yamlFields(reflect.TypeOf(guideapi.Step{}))

// knownTargetFields are a target's fields: the anchor and the instance. A role
// or a label is refused by name — they are GUI text, which is translated
// (modules/guide, decision OD-007).
var knownTargetFields = yamlFields(reflect.TypeOf(guideapi.Target{}))

// yamlFields are the YAML keys a struct type decodes.
func yamlFields(t reflect.Type) map[string]bool {
	out := map[string]bool{}
	for i := 0; i < t.NumField(); i++ {
		name, _, _ := strings.Cut(t.Field(i).Tag.Get("yaml"), ",")
		if name != "" && name != "-" {
			out[name] = true
		}
	}
	return out
}

// LintRecipe checks a recipe file against the guide-recipe contract.
func LintRecipe(path string) verify.LintResult {
	data, err := os.ReadFile(path)
	if err != nil {
		return verify.ErrorResult("unreadable", "guide-recipe: cannot read "+path)
	}
	return LintRecipeBytes(data)
}

// LintRecipeBytes is LintRecipe over content already in hand.
func LintRecipeBytes(data []byte) verify.LintResult {
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil || raw == nil {
		return verify.Result(errf("yaml-invalid", "the recipe is not a YAML mapping"))
	}
	r, err := guideapi.ParseRecipe(data)
	if err != nil {
		return verify.Result(errf("yaml-invalid", "%v", err))
	}
	var f []verify.LintFinding
	for k := range raw {
		if !knownFields[k] {
			f = append(f, errf("unknown-field", "unknown field %q (a typo is silently dropped)", k))
		}
	}
	if strings.TrimSpace(r.Name) == "" {
		f = append(f, errf("name-missing", "`name` (the title the picker shows) is required"))
	}
	if strings.TrimSpace(r.Description) == "" {
		f = append(f, warnf("description-missing", "`description` says in one sentence what the recipe achieves"))
	}
	if len(r.Hashtags) == 0 {
		f = append(f, warnf("hashtags-missing", "`hashtags` are the words a person types to find the recipe"))
	}
	switch r.Level {
	case "", guideapi.LevelBeginner, guideapi.LevelRegular, guideapi.LevelExpert:
	default:
		f = append(f, errf("level-invalid", "level must be beginner | regular | expert, got %q", r.Level))
	}
	for _, req := range r.Requires {
		if !reIdentity.MatchString(req) {
			f = append(f, errf("requires-invalid", "requires %q is not a recipe identity (a kebab-case file name without extension)", req))
		}
	}
	for _, rel := range r.Related {
		if _, err := contribapi.ParseResourceURI(rel); err != nil {
			f = append(f, errf("related-invalid", "related %q is not a resource URI: %v", rel, err))
		}
	}
	if r.Video != "" && !strings.HasPrefix(r.Video, "https://") && !strings.HasPrefix(r.Video, "http://") {
		f = append(f, errf("video-invalid", "video must be a link (http or https), got %q", r.Video))
	}
	if r.ShownOnce < 0 {
		f = append(f, errf("shown-once-negative", "`shown_once` is a place in the start queue: 1 or more (absent: never)"))
	}
	if r.OnceKey != "" && r.ShownOnce == 0 {
		f = append(f, warnf("once-key-unused", "`once_key` matters only to a guide shown once at start (`shown_once`)"))
	}
	if r.Nature() == "" {
		f = append(f, errf("guide-empty", "a guide holds steps (a Help me), a video (a Show me) or a markdown body (an Explain me)"))
	}
	rawSteps, _ := raw["steps"].([]any)
	seen := map[string]int{}
	for i, s := range r.Steps {
		n := i + 1
		if i < len(rawSteps) {
			if m, ok := rawSteps[i].(map[string]any); ok {
				for k := range m {
					if !knownStepFields[k] {
						f = append(f, errf("step-unknown-field", "step %d: unknown field %q", n, k))
					}
				}
				if tm, ok := m["target"].(map[string]any); ok {
					for k := range tm {
						switch {
						case k == "role" || k == "label":
							f = append(f, errf("step-target-by-text", "step %d: a target names its control by `guide` anchor, not by %s — GUI text is translated", n, k))
						case !knownTargetFields[k]:
							f = append(f, errf("step-target-unknown-field", "step %d: unknown target field %q", n, k))
						}
					}
				}
			}
		}
		switch {
		case s.ID == "":
			f = append(f, errf("step-id-missing", "step %d: `id` (the stable key its translations name) is required", n))
		case !reIdentity.MatchString(s.ID):
			f = append(f, errf("step-id-invalid", "step %d: id %q must be kebab-case", n, s.ID))
		case seen[s.ID] > 0:
			f = append(f, errf("step-id-duplicate", "step %d: id %q is already step %d's", n, s.ID, seen[s.ID]))
		default:
			seen[s.ID] = n
		}
		f = append(f, lintStep(n, s)...)
	}
	if len(r.Steps) > 12 {
		f = append(f, warnf("too-long", "%d steps: a recipe longer than about ten steps is a documentation page to link to", len(r.Steps)))
	}
	return verify.Result(f...)
}

func lintStep(n int, s guideapi.Step) []verify.LintFinding {
	var f []verify.LintFinding
	if strings.TrimSpace(s.Say) == "" {
		f = append(f, errf("step-say-missing", "step %d: `say` (what the step tells the person) is required", n))
	}
	f = append(f, LintStepText(n, s.Say)...)
	switch s.Await {
	case "", guideapi.AwaitManual:
	case guideapi.AwaitClick:
		if s.Target == nil {
			f = append(f, errf("step-click-without-target", "step %d: `await: click` needs a `target` to click", n))
		}
	case guideapi.AwaitRoute:
		if s.Route == "" {
			f = append(f, errf("step-route-missing", "step %d: `await: route` needs `route` (the pane or tab that completes it)", n))
		}
	default:
		f = append(f, errf("step-await-invalid", "step %d: await must be click | route | manual, got %q", n, s.Await))
	}
	if s.Route != "" && s.Await != guideapi.AwaitRoute {
		f = append(f, errf("step-route-without-await", "step %d: `route` is only read with `await: route`", n))
	}
	if t := s.Target; t != nil && t.Guide == "" {
		f = append(f, errf("step-target-empty", "step %d: a target names its control's `guide` anchor", n))
	}
	for field, uri := range map[string]string{"do": s.Do, "check": s.Check} {
		if uri == "" {
			continue
		}
		if _, err := mbapi.ParseToolURI(uri); err != nil {
			f = append(f, errf("step-uri-invalid", "step %d: %s %q is not an action URI: %v", n, field, uri, err))
		}
	}
	if len(s.With) > 0 && s.Do == "" && s.Check == "" {
		f = append(f, errf("step-with-unused", "step %d: `with` is passed to a check or a do action, and the step has neither", n))
	}
	if s.Do != "" && s.Check != "" {
		f = append(f, errf("step-do-and-check", "step %d: a step either does something or checks it, not both", n))
	}
	return f
}

// reBold is a name in bold: the form a step's text named a control by before
// controls were named by anchor.
var reBold = regexp.MustCompile(`\*\*([^*]+)\*\*`)

// LintStepText judges what a step says (its English `say`, or a translation
// of it): a control is named `{control:<anchor>}` (`{control:<anchor>|<name>}`
// for an instance in a list), which the player replaces with the label the
// GUI shows in the person's language — never by its label written in bold,
// which is GUI text and is translated (modules/guide, decision OD-007). Whether
// the anchor exists is the GUI's registry's to say: the guide's Go test holds
// the shipped recipes to it (internal/test/guide), as a sidecar's build cannot
// read the GUI.
func LintStepText(n int, text string) []verify.LintFinding {
	var f []verify.LintFinding
	if _, malformed := guideapi.ControlRefs(text); malformed > 0 {
		f = append(f, errf("step-control-malformed", "step %d: a control is named {control:<anchor>} or {control:<anchor>|<name>} — kebab-case anchor, nothing else inside the braces", n))
	}
	for _, m := range reBold.FindAllStringSubmatch(text, -1) {
		f = append(f, errf("step-control-by-label", "step %d: **%s** names a control by its label; name it {control:<anchor>} so the player says the GUI's own label in the person's language (a term is in italics)", n, m[1]))
	}
	return f
}

// Action is what LintChecks needs to know about an action URI — the subset of
// ActionService.Describe it reads.
type Action struct {
	ReadOnly     bool
	IsAsync      bool
	OutputSchema json.RawMessage
}

// Describer describes an action URI (ActionService.Describe in a universe).
type Describer func(ctx context.Context, uri string) (Action, error)

// LintChecks judges every `check` a recipe names against the universe: a
// method that declares itself read-only, runs synchronously and answers `ok`
// and `detail`. A check that cannot be described is an error too — a recipe
// must not name what the universe does not publish.
func LintChecks(ctx context.Context, r guideapi.Recipe, describe Describer) verify.LintResult {
	var f []verify.LintFinding
	for i, s := range r.Steps {
		if s.Check == "" {
			continue
		}
		n := i + 1
		a, err := describe(ctx, s.Check)
		if err != nil {
			f = append(f, errf("check-unknown", "step %d: check %q cannot be described: %v", n, s.Check, err))
			continue
		}
		if !a.ReadOnly {
			f = append(f, errf("check-not-read-only", "step %d: check %q does not declare itself read-only (meta.MethodDef.ReadOnly)", n, s.Check))
		}
		if a.IsAsync {
			f = append(f, errf("check-async", "step %d: check %q runs asynchronously; a check answers at once", n, s.Check))
		}
		if !hasOKAndDetail(a.OutputSchema) {
			f = append(f, errf("check-output", "step %d: check %q must answer `ok` (boolean) and `detail` (string)", n, s.Check))
		}
	}
	return verify.Result(f...)
}

// hasOKAndDetail reports whether a JSON Schema output declares `ok` as a
// boolean and `detail` as a string.
func hasOKAndDetail(schema json.RawMessage) bool {
	var s struct {
		Properties map[string]struct {
			Type any `json:"type"`
		} `json:"properties"`
	}
	if len(schema) == 0 || json.Unmarshal(schema, &s) != nil {
		return false
	}
	return typeIs(s.Properties["ok"].Type, "boolean") && typeIs(s.Properties["detail"].Type, "string")
}

func typeIs(t any, want string) bool {
	switch v := t.(type) {
	case string:
		return v == want
	case []any:
		for _, x := range v {
			if x == want {
				return true
			}
		}
	}
	return false
}

func errf(rule, format string, a ...any) verify.LintFinding {
	return verify.LintFinding{Rule: rule, Message: fmt.Sprintf(format, a...), Severity: verify.SeverityError}
}

func warnf(rule, format string, a ...any) verify.LintFinding {
	return verify.LintFinding{Rule: rule, Message: fmt.Sprintf(format, a...), Severity: verify.SeverityWarn}
}
