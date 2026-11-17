// SPDX-License-Identifier: Apache-2.0

package api

import "strings"

// ContributionTypeNotation is the Type discriminator of a notation.
const ContributionTypeNotation = "notation"

// NotationBody is a `notation` contribution: the language a diagram
// speaks — its element kinds, relationship kinds, and the rules that make
// a model correct for it. It says nothing about looks (drawio-template)
// or about which kinds one picture shows (viewpoint).
type NotationBody struct {
	ID            string                      `json:"id"                     jsonschema:"description=Kebab-case identifier; matches the contribution ID (the body file name without extension)"`
	DisplayName   string                      `json:"displayName"            jsonschema:"description=Human-readable name such as ArchiMate 3.2 or UML 2.5"`
	Version       string                      `json:"version,omitempty"      jsonschema:"description=Version of the language this notation follows"`
	Description   string                      `json:"description,omitempty"  jsonschema:"description=One-paragraph description for a picker"`
	Elements      map[string]ElementKind      `json:"elements"               jsonschema:"description=Element kinds by key; a node's kind is one of these keys"`
	Relationships map[string]RelationshipKind `json:"relationships"          jsonschema:"description=Relationship kinds by key; an edge's kind is one of these keys"`
	Rules         *NotationRules              `json:"rules,omitempty"        jsonschema:"description=What makes a model correct beyond known kinds; absent means any relationship joins any elements"`
}

// ElementKind is one element kind of a notation.
type ElementKind struct {
	Label       string `json:"label"                 jsonschema:"description=Human-readable kind name"`
	Description string `json:"description,omitempty" jsonschema:"description=What an element of this kind stands for"`
	Category    string `json:"category,omitempty"    jsonschema:"description=Grouping of kinds (an ArchiMate layer or aspect); rules name a category as category:<name>"`
}

// RelationshipKind is one relationship kind of a notation.
type RelationshipKind struct {
	Label       string `json:"label"                 jsonschema:"description=Human-readable relationship name"`
	Description string `json:"description,omitempty" jsonschema:"description=What the relationship means"`
}

// NotationRules is what a notation allows.
type NotationRules struct {
	// Allowed lists the permitted relationships. When it is empty every
	// relationship may join any two elements; when it is not, an edge is
	// correct only if some rule allows its (source, kind, target).
	Allowed []RelationshipRule `json:"allowed,omitempty" jsonschema:"description=Permitted relationships; empty allows every relationship between any elements"`
}

// RelationshipRule allows one relationship kind from any of Sources to
// any of Targets. An entry is an element kind, `category:<name>`, or `*`.
type RelationshipRule struct {
	Relationship string   `json:"relationship" jsonschema:"description=The relationship kind this rule allows (or * for all)"`
	Sources      []string `json:"sources"      jsonschema:"description=Element kinds or category:<name> or * the relationship may start from"`
	Targets      []string `json:"targets"      jsonschema:"description=Element kinds or category:<name> or * the relationship may end at"`
}

// Allows reports whether the notation's rules permit a relationship of
// kind rel from an element of kind src to one of kind tgt. Kinds the
// notation does not declare are not this method's concern (validation
// reports them separately) and are allowed here.
func (n *NotationBody) Allows(src, rel, tgt string) bool {
	if n.Rules == nil || len(n.Rules.Allowed) == 0 {
		return true
	}
	if _, ok := n.Elements[src]; !ok {
		return true
	}
	if _, ok := n.Elements[tgt]; !ok {
		return true
	}
	if _, ok := n.Relationships[rel]; !ok {
		return true
	}
	for _, r := range n.Rules.Allowed {
		if (r.Relationship == rel || r.Relationship == "*") && n.matches(r.Sources, src) && n.matches(r.Targets, tgt) {
			return true
		}
	}
	return false
}

func (n *NotationBody) matches(entries []string, kind string) bool {
	for _, e := range entries {
		switch {
		case e == "*" || e == kind:
			return true
		case strings.HasPrefix(e, "category:"):
			if el, ok := n.Elements[kind]; ok && el.Category == strings.TrimPrefix(e, "category:") {
				return true
			}
		}
	}
	return false
}
