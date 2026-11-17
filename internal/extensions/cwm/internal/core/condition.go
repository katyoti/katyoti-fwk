// SPDX-License-Identifier: Apache-2.0

package core

import (
	"errors"
	"fmt"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/el"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// Expressions — action `when:` predicates, task `filter:` predicates and `${...}`
// input placeholders — are el (internal/core/el, expr-lang), the ONE expression
// language of the system (decided 2026-09-25, p7o). CEL was here only because
// this feature predates el; it is gone, and so is the case where one TASK.md
// `filter:` was read by el in one place and by CEL in another.
//
// ONE VOCABULARY, a declared environment: object (capability groups, + body),
// id, type, parent_id, children. A name outside it is a compile error naming
// it ("unknown name node"), which says the expression speaks another
// vocabulary rather than reading false forever.
//
// Examples: `object.content?.status == 'ready'`, `'x' in object.fields`,
// `any(children, .type == 'todo' && .status == 'done')`.

var (
	// The object vocabulary is declared ONCE, in el, because the task loader
	// compiles `filter:` against it too.
	objectVocabulary = el.ObjectVocabulary()

	objectExprs = el.NewEnvCache(objectVocabulary)
)

// evalPredicate evaluates a `when:` / `filter:` in a vocabulary. Fail closed
// and visible: a predicate that does not compile, errors, or returns a non-bool
// DISABLES the action with the error as the reason — a broken guard must never
// silently enable a gated action.
func evalPredicate(exprs *el.Cache, expr string, env map[string]any) (disabled bool, reason string) {
	if _, err := exprs.Compile(expr); err != nil {
		return true, fmt.Sprintf("when-predicate error: %v", err)
	}
	ok, err := exprs.EvalPredicate(expr, env)
	if errors.Is(err, el.ErrNotBool) {
		return true, "when-predicate did not return a bool"
	}
	if err != nil {
		return true, fmt.Sprintf("when-predicate eval failed: %v", err)
	}
	if !ok {
		return true, fmt.Sprintf("condition not met: %s", expr)
	}
	return false, ""
}

// objectConditionEnv projects an object onto the object vocabulary.
//
// THE BINDING DECISION (cwm.p1r, taken 2026-09-19): capability groups under
// `object`, identity as top-level scalars. The alternative — a flattened,
// node-shaped view for source compatibility — was rejected: buying
// compatibility by flattening the model is how the node/item shape survives
// the cut meant to remove it, and exactly ONE shipped declaration writes a
// `node.*` predicate today, so there is almost nothing to be compatible with.
//
// One binding is the point. A reader who knows `object.checkable.probe` from a
// probe (evaluated through ObjectEvaluate) knows `object.content.status` in a
// guard; two bindings for one model would be two things to learn and one to get
// wrong.
func objectConditionEnv(obj api.Object) map[string]any {
	caps := map[string]any{}
	for id, group := range obj.Capabilities {
		// Copied into a fresh map[string]any: the stored shape is
		// map[string]map[string]any, and the evaluator needs the value side to be dyn.
		fields := map[string]any{}
		for k, v := range group {
			fields[k] = v
		}
		caps[id] = fields
		// A hyphen ends a name in an expression (`object.module-health` is a
		// subtraction), so a hyphenated group also reads with underscores:
		// `object.module_health.score`. Ids stay kebab everywhere else.
		if alias := strings.ReplaceAll(id, "-", "_"); alias != id {
			caps[alias] = fields
		}
	}
	// The object core, by name (`object.name`, `object.parent`).
	caps["name"] = obj.Name
	caps["description"] = obj.Description
	caps["icon"] = obj.Icon
	caps["parent"] = obj.Parent
	caps["slug"] = obj.Slug
	caps["tags"] = stringsAny(obj.Tags)
	if obj.Body != "" {
		caps["body"] = obj.Body
	}
	if len(obj.Fields) > 0 {
		caps["fields"] = obj.Fields // the type's own fields: `object.fields.<name>`
	}
	// The object's own named files, by name (`object.files.screenshot.path`),
	// always present — empty when it names none — so `len(object.files) > 0`
	// reads on every object.
	files := make(map[string]any, len(obj.Files))
	for name, f := range obj.Files {
		files[name] = map[string]any{"path": f.Path, "media": f.Media}
	}
	caps["files"] = files
	return caps
}

func stringsAny(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}

// evalObjectActionWhen is evalActionWhen for an object — same contract, same
// failure posture.
//
// A predicate that errors, fails to compile, or returns a non-bool DISABLES the
// action rather than enabling it. A broken guard must never read as permission:
// the whole reason a `when:` exists is to withhold an action, so the failure
// mode has to be withholding it.
func evalObjectActionWhen(expr string, obj api.Object) (disabled bool, reason string) {
	// The node-era variables are deliberately NOT in the object vocabulary: a
	// not-yet-migrated `node.*` predicate is refused at compile with
	// "unknown name node", which names the VOCABULARY — the reader is told the
	// predicate still speaks node, not that one field is missing.
	return evalPredicate(objectExprs, expr, map[string]any{
		"object":    objectConditionEnv(obj),
		"id":        obj.ID,
		"type":      obj.Type,
		"parent_id": obj.ParentID(),
		"children":  []map[string]any{},
	})
}
