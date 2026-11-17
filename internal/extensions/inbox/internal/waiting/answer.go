// SPDX-License-Identifier: Apache-2.0

package waiting

import (
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	"github.com/katyoti/katyoti-fwk/internal/extensions/humangate/formdef"
	"github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
)

// THE HUMAN-TASK CARD'S ANSWER AREA - what answers a question
// by its kind, as the approved mock-up draws it
// (modules/research/human-interaction/human-interaction-mockup.html): a
// verdict is two buttons - « Approuver » (« (passkey) » when that proof is
// asked) and « Refuser », which asks why -, a test by hand « Tout est bon :
// je valide » or « Quelque chose a échoué »; a choice its options in a
// drop-down and « Choisir »; a form the form it asks; a permission its kind's
// answer data-type (allow or deny, once or always, and the rule an always
// keeps among the rules the tool proposes); data to provide or an answer to
// give a text. « Je le prends » sits beside them on a holding list's question
// no one took. Each answer reports the human task's to-do; every element
// carries an id a test finds it by.

// humanTaskAnswer is a human task's answer area: what reports its to-do.
func humanTaskAnswer(it api.WaitingItem, lang string) []node {
	var take []node
	if it.List != "" && it.ClaimedBy == nil {
		take = append(take, render.Button(L(lang, "take"), render.Action{ID: "take", Label: L(lang, "take"), Icon: "hand-grab", URI: todosPullURI,
			Args: map[string]any{"id": it.Todo, "by": "${person}"}}).With("id", "take:"+it.Todo))
	}
	report := func(args map[string]any) map[string]any {
		out := map[string]any{"id": it.Todo, "status": "success", "by": "${person}"}
		for k, v := range args {
			out[k] = v
		}
		return out
	}
	switch {
	case it.Kind == api.KindPermission:
		return append(row(take), permissionAnswer(it, lang, report))
	case verdictKinds[it.Kind]:
		yes, no, why := "approve", "reject", "why"
		if it.Kind == api.KindTestByHand {
			yes, no, why = "all_good", "failed", "what_failed"
		}
		return []node{verdictButtons(it.Todo, lang, yes, no, why, it.Proof,
			render.RunURI(L(lang, yes), todosReportURI, report(map[string]any{"verdict": "approved", "summary": L(lang, yes)})),
			render.RunURI(L(lang, no), todosReportURI, report(map[string]any{"verdict": "rejected", "summary": "${value.note}"})),
			take...)}
	case it.Kind == api.KindForm && it.FormDef != "":
		if s, ok := formSchema(it.FormDef); ok {
			// Its answer is the form's data.
			return append(row(take), render.AnswerForm(s, render.RunURI(L(lang, "send"), todosReportURI,
				report(map[string]any{"summary": L(lang, "form_given"), "more": map[string]any{"data": "${value}"}})),
				"send", map[string]any{}).With("id", "answer:"+it.Todo))
		}
		// A form its definition cannot draw: its fields shown, answered in words.
		return append(row(take), formAsked(it.FormDef, lang), textAnswer(it, lang, report))
	case len(it.Options) > 0:
		// A choose question offers its own choices: its answer is one of them
		// (a decision says why).
		labels := map[string]string{"choice": L(lang, "choice")}
		args := map[string]any{"answer": "${value.choice}", "summary": "${value.choice}"}
		var s map[string]any
		if it.Kind == api.KindDecide {
			labels["note"] = L(lang, "note")
			args["details"] = "${value.note}"
			s = choiceSchema[choiceForm]("Choice", lang, it.Options, labels)
		} else {
			s = choiceSchema[chooseForm]("Choice", lang, it.Options, labels)
		}
		return append(row(take), render.AnswerForm(s, render.RunURI(L(lang, "choose"), todosReportURI, report(args)),
			"check", map[string]any{"choice": it.Options[0]}).With("id", "answer:"+it.Todo))
	}
	return append(row(take), textAnswer(it, lang, report))
}

// row is the buttons in one wrapping row; none, nothing.
func row(buttons []node) []node {
	if len(buttons) == 0 {
		return nil
	}
	return []node{{"kind": "hbox", "gap": 8, "wrap": true, "children": buttons}}
}

// verdictButtons is a verdict as the mock-up draws it: the approval filled
// first, the rejection asking why in a modal, then the extra buttons.
func verdictButtons(id, lang, yes, no, why, proof string, approve, reject render.Action, extra ...node) node {
	label := L(lang, yes)
	if proof == "passkey" {
		label += L(lang, "passkey")
	}
	approve.ID, approve.Label, approve.Icon = yes, label, "check"
	ask := render.AnswerForm(hinted(render.InlineSchema[whyForm]("Why", map[string]string{"note": L(lang, why)}), lang, map[string]string{"note": L(lang, why)}), reject, "x", whyForm{}).
		With("id", "why:"+id)
	open := render.Action{ID: no, Label: L(lang, no), Icon: "x", Modal: &render.Modal{Title: L(lang, no), Node: ask, Size: "sm"}}
	buttons := []node{
		render.Button(label, approve).With("variant", "filled").With("size", "sm").With("id", yes+":"+id),
		render.Button(L(lang, no), open).With("variant", "default").With("size", "sm").With("id", no+":"+id),
	}
	return node{"kind": "hbox", "gap": 8, "wrap": true, "children": append(buttons, extra...)}
}

// permissionAnswer is a permission's answer: its kind's answer data-type
// (api.PermissionAnswer, published as ktw-agents:/data-type/permission-answer),
// its rule one of the rules the tool proposes; the decision is the answer and
// the whole the answer's data, as the runtime reads them.
func permissionAnswer(it api.WaitingItem, lang string, report func(map[string]any) map[string]any) node {
	s := hinted(render.InlineSchema[api.PermissionAnswer]("PermissionAnswer", map[string]string{"decision": L(lang, "decision"), "rule": L(lang, "rule")}),
		lang, map[string]string{"decision": L(lang, "h_decision"), "rule": L(lang, "h_rule")})
	data := map[string]any{"decision": api.PermissionAllowOnce}
	if len(it.Options) > 0 {
		setEnum(s, "PermissionAnswer", "rule", it.Options)
		data["rule"] = it.Options[0]
	}
	return render.AnswerForm(s, render.RunURI(L(lang, "send"), todosReportURI,
		report(map[string]any{"answer": "${value.decision}", "summary": "${value.decision}", "more": "${value}"})),
		"send", data).With("id", "answer:"+it.Todo)
}

// textAnswer is an answer in words.
func textAnswer(it api.WaitingItem, lang string, report func(map[string]any) map[string]any) node {
	return render.AnswerForm(hinted(render.InlineSchema[textForm]("Answer", map[string]string{"note": L(lang, "answer")}), lang, map[string]string{"note": L(lang, "answer")}),
		render.RunURI(L(lang, "send"), todosReportURI, report(map[string]any{"answer": "${value.note}", "summary": "${value.note}"})),
		"send", textForm{}).With("id", "answer:"+it.Todo)
}

// formAsked shows a form's definition when it cannot be drawn.
func formAsked(def, lang string) node {
	return node{"kind": "markdown", "content": L(lang, "form_asked") + "\n\n```yaml\n" + trimNL(def) + "\n```", "maxHeight": 240}
}

func trimNL(s string) string {
	for len(s) > 0 && s[len(s)-1] == '\n' {
		s = s[:len(s)-1]
	}
	return s
}

// whyForm is why a verdict rejects.
type whyForm struct {
	Note string `json:"note" jsonschema:"description=Why,required"`
}

// chooseForm is a choice among a question's options.
type chooseForm struct {
	Choice string `json:"choice" jsonschema:"description=Your choice,required"`
}

// choiceSchema is T's schema whose choice is one of the question's options.
func choiceSchema[T any](root, lang string, options []string, labels map[string]string) map[string]any {
	s := hinted(render.InlineSchema[T](root, labels), lang, labels)
	setEnum(s, root, "choice", options)
	return s
}

// setEnum makes a field of an inline schema's root one of values.
func setEnum(s map[string]any, root, field string, values []string) {
	types, _ := s["types"].(map[string]any)
	t, _ := types[root].(map[string]any)
	fields, _ := t["fields"].(map[string]any)
	f, ok := fields[field].(map[string]any)
	if !ok {
		return
	}
	enum := make([]any, 0, len(values))
	for _, v := range values {
		enum = append(enum, v)
	}
	f["enum"] = enum
}

// formSchema is a form question's FormDefinition as an inline schema - its
// fields, labels, choices and required ones; ok is false for a definition
// that does not read or holds a field a flat form cannot draw (an object, a
// list, a table).
func formSchema(def string) (map[string]any, bool) {
	fd, err := formdef.Parse([]byte(def))
	if err != nil {
		return nil, false
	}
	fields := map[string]any{}
	for _, f := range fd.AllFields() {
		out := map[string]any{}
		switch f.Type {
		case "string", "text", "":
			out["type"] = "string"
		case "markdown":
			out["type"], out["content"] = "string", "markdown"
		case "date":
			out["type"], out["content"] = "string", "date"
		case "time":
			out["type"], out["content"] = "string", "time"
		case "datetime":
			out["type"], out["content"] = "string", "date-time"
		case "integer", "number":
			out["type"] = "number"
		case "boolean":
			out["type"] = "boolean"
		case "enum":
			out["type"] = "string"
			enum := make([]any, 0, len(f.Options))
			for _, o := range f.Options {
				enum = append(enum, o.Value)
			}
			out["enum"] = enum
		default:
			return nil, false
		}
		if f.Label != "" {
			out["label"] = f.Label
		}
		if f.Description != "" {
			out["description"] = f.Description
		}
		if !f.Required {
			out["optional"] = true
		}
		fields[f.Key] = out
	}
	if len(fields) == 0 {
		return nil, false
	}
	return map[string]any{"root": "Form", "types": map[string]any{"Form": map[string]any{"type": "object", "fields": fields}}}, true
}

// hinted is a schema whose fields' hints are in the person's language: in
// French the words given - none where they only repeat the field's label -;
// in English its fields' own.
func hinted(s map[string]any, lang string, hints map[string]string) map[string]any {
	if lang != "fr" {
		return s
	}
	labels := map[string]string{}
	if types, ok := s["types"].(map[string]any); ok {
		if t, ok := types[s["root"].(string)].(map[string]any); ok {
			if fields, ok := t["fields"].(map[string]any); ok {
				for name, f := range fields {
					if fm, ok := f.(map[string]any); ok {
						labels[name], _ = fm["label"].(string)
					}
				}
			}
		}
	}
	out := map[string]string{}
	for field, h := range hints {
		if h == labels[field] {
			h = "" // the label says it: no hint repeating it
		}
		out[field] = h
	}
	return render.DescribeFields(s, out)
}
