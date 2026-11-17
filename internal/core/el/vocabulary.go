// SPDX-License-Identifier: Apache-2.0

package el

// ObjectVocabulary is the environment every expression over ONE object sees,
// whichever package evaluates it: an object type's action `when:`, a task's
// `filter:`, a `${...}` input placeholder.
//
//	object     the object's core (`object.name`, `object.body` — its prose),
//	           its capability groups (`object.lifecycle.state`; a hyphenated
//	           one also with underscores, `object.module_health.score`) and
//	           its type's own fields (`object.fields.<key>`)
//	id         the object id
//	type       the object type
//	parent_id  the container parent's id
//	children   what hangs under the object: id / type / title / status / tags
//	links      the object's incident edges: kind / direction (out | in) / target
//	current    the minimal projection: id, name, type, type_name, icon,
//	           icon_name, parent, module, file, capabilities (the ids of the
//	           effective capabilities) (+ capability groups by id)
//
// It lives here, and not with the object model, because two packages that must
// not import each other both compile against it: the task loader refuses a
// `filter:` that names anything else, and CWM evaluates it. One declaration is
// what makes "the same filter means the same thing in both" true by
// construction.
//
// Returns a fresh map; pass it to NewEnvCache.
func ObjectVocabulary() map[string]any {
	return map[string]any{
		"object":    map[string]any{},
		"id":        "",
		"type":      "",
		"parent_id": "",
		"children":  []any{},
		"links":     []any{},
		"current":   map[string]any{},
	}
}

// HookVocabulary is what a hook entry's `when`, its filter's `when`, its
// `input` and an async entry's `key` see: the object vocabulary, plus
//
//	event   the event the point fires on: {type, id, at, cause, payload} —
//	        type the event type's name (object.updated, code.changed), id its
//	        event id, at its time (RFC 3339), cause {kind, id, depth} or nil,
//	        payload what the raiser sent (empty for an object write)
//
// so `'go.mod' in event.payload.paths` is how a subscription to a code change
// narrows to the changes that touch go.mod.
func HookVocabulary() map[string]any {
	v := ObjectVocabulary()
	v["event"] = map[string]any{}
	return v
}

// WithFunctions declares each name in vocab as a function an expression may
// call — `gate('ready')`. The value is a stand-in for compiling; the evaluator
// binds the real call, of the same signature, in the environment it passes.
// Returns vocab.
func WithFunctions(vocab map[string]any, names []string) map[string]any {
	for _, n := range names {
		vocab[n] = func(args ...any) (any, error) { return nil, nil }
	}
	return vocab
}

// TransitionVocabulary is what a workflow transition's gate sees: the object
// vocabulary, plus
//
//	by      the actor making the transition: {id, kind} — kind is person or agent
//	from    the state the object is in
//	to      the state it would move to
//
// so `by.kind == 'person'` is how a workflow says only a person may accept.
// What a guard asks a service — `verify().green && lint().blocking == 0` — is a
// function a capability adds (WithFunctions).
func TransitionVocabulary() map[string]any {
	v := ObjectVocabulary()
	v["by"] = map[string]any{}
	v["from"] = ""
	v["to"] = ""
	return v
}
