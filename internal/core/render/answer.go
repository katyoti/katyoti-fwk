// SPDX-License-Identifier: Apache-2.0

package render

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// The one answer: what a page source, a section's load, an
// action's result and a display panel's opening event carry, and the one op
// set that patches any node of a page by its id, pulled or pushed
// (modules/display/page-dsl.md « Sections, answers and ops »).

// Spec is one part of a specification - a page tree, a data type, a ui
// variant - given inline or named by its contribution URI and the sha256 of
// its body. A host keeps every part named by URI in one cache keyed by URI and
// hash, so a part already held costs nothing and only data crosses the wire;
// an inline part is never cached. On the wire an inline part is the part
// itself; a named one is {"uri", "hash"}.
type Spec struct {
	URI    string `json:"uri,omitempty"`
	Hash   string `json:"hash,omitempty"`
	Inline any    `json:"-"`
}

// SpecOf names a part by its URI and the hash of its body.
func SpecOf(uri, hash string) Spec { return Spec{URI: uri, Hash: hash} }

// InlineSpec gives a part inline: a page tree (a Node), a data type's AST, a
// ui block.
func InlineSpec(v any) Spec { return Spec{Inline: v} }

// SpecHash is the hash a part is named with: "sha256:" and the hex sha256 of
// its JSON. encoding/json writes map keys sorted, so a part decoded and
// written again - its numbers kept as written (json.Number) - hashes the
// same. The authority answering a part by URI (object-pane) and a producer
// naming one it holds compute it alike.
func SpecHash(part any) (string, error) {
	raw, err := json.Marshal(part)
	if err != nil {
		return "", fmt.Errorf("render: hash a part: %w", err)
	}
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// NamedSpec names a part the producer holds by its URI and the hash of its
// body. A producer that does not hold the body names it by SpecOf(uri, "")
// and the host learns the hash when it resolves the part.
func NamedSpec(uri string, part any) (Spec, error) {
	h, err := SpecHash(part)
	if err != nil {
		return Spec{}, err
	}
	return Spec{URI: uri, Hash: h}, nil
}

// Named reports whether the part is named by URI (and so cacheable).
func (s Spec) Named() bool { return s.URI != "" }

// MarshalJSON writes a named part as {"uri","hash"} and an inline one as the
// part itself.
func (s Spec) MarshalJSON() ([]byte, error) {
	if s.Named() {
		return json.Marshal(struct {
			URI  string `json:"uri"`
			Hash string `json:"hash,omitempty"`
		}{s.URI, s.Hash})
	}
	return json.Marshal(s.Inline)
}

// UnmarshalJSON reads {"uri", "hash"} (and nothing else) as a named part;
// anything else is the inline part.
func (s *Spec) UnmarshalJSON(b []byte) error {
	var probe map[string]json.RawMessage
	if json.Unmarshal(b, &probe) == nil && probe["uri"] != nil {
		named := len(probe) == 1 || (len(probe) == 2 && probe["hash"] != nil)
		if named {
			var r struct {
				URI  string `json:"uri"`
				Hash string `json:"hash"`
			}
			if err := json.Unmarshal(b, &r); err != nil {
				return err
			}
			*s = Spec{URI: r.URI, Hash: r.Hash}
			return nil
		}
	}
	var v any
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	if err := dec.Decode(&v); err != nil {
		return err
	}
	*s = Spec{Inline: v}
	return nil
}

// Answer is what a page source, a section's load, an action's result and a
// display panel's opening event answer: Tree, the page tree (a page root, or
// one node for a section); Parts, the data types and ui variants the tree
// references, by the reference the tree uses; Data, what the tree shows. Panel
// instead of a tree: the answer is a display panel, whose events carry the
// same Answer and the same ops.
type Answer struct {
	Tree  *Spec           `json:"tree,omitempty"`
	Parts map[string]Spec `json:"parts,omitempty"`
	Data  any             `json:"data,omitempty"`
	Panel string          `json:"panel,omitempty"`
}

// Validate refuses an answer that is both a tree and a panel, or neither
// with no data, and an inline tree off the page grammar.
func (a Answer) Validate() error {
	if a.Panel != "" && a.Tree != nil {
		return fmt.Errorf("render: answer: a panel or a tree, not both")
	}
	if a.Panel == "" && a.Tree == nil && a.Data == nil {
		return fmt.Errorf("render: answer: a tree, data or a panel")
	}
	for ref, p := range a.Parts {
		if !p.Named() && p.Inline == nil {
			return fmt.Errorf("render: answer: part %q is empty", ref)
		}
	}
	if a.Tree != nil && !a.Tree.Named() {
		if a.Tree.Inline == nil {
			return fmt.Errorf("render: answer: an empty tree")
		}
		return ValidatePage(a.Tree.Inline)
	}
	return nil
}

// OpKind is what an op does to the node it targets.
type OpKind string

const (
	// OpReplace replaces the target node by Node (a section's loaded content).
	OpReplace OpKind = "replace"
	// OpAppend appends Items to the target's data (a list, table, tree or lanes).
	OpAppend OpKind = "append"
	// OpMerge merges Value into the target node's fields (RFC 7396).
	OpMerge OpKind = "merge"
	// OpRemove removes the target node.
	OpRemove OpKind = "remove"
	// OpFail replaces only the target by an error card with Message and « Réessayer ».
	OpFail OpKind = "fail"
	// OpComplete ends the page's loading state; it takes no target.
	OpComplete OpKind = "complete"
)

// Op is one change to a page by node id, the same for a pulled section, a
// live patch and a display panel's event. Ops apply in the order of Seq per
// page or panel. Target is a node id, unique in the page and stable across
// rebuilds (`lane:<stream>`, `tab:<name>`), never a JSON Pointer.
type Op struct {
	Seq     int64  `json:"seq"`
	Op      OpKind `json:"op"`
	Target  string `json:"target,omitempty"`
	Node    Node   `json:"node,omitempty"`
	Items   []any  `json:"items,omitempty"`
	Value   any    `json:"value,omitempty"`
	Message string `json:"message,omitempty"`
}

// Validate refuses an op of an unknown kind, one with no target (but
// complete), and one missing what its kind needs.
func (o Op) Validate() error {
	switch o.Op {
	case OpComplete:
		return nil
	case OpReplace, OpAppend, OpMerge, OpRemove, OpFail:
	default:
		return fmt.Errorf("render: op %d: unknown op %q", o.Seq, o.Op)
	}
	if o.Target == "" {
		return fmt.Errorf("render: op %d: %s needs its target node id", o.Seq, o.Op)
	}
	switch o.Op {
	case OpReplace:
		if o.Node == nil {
			return fmt.Errorf("render: op %d: replace needs its node", o.Seq)
		}
		return ValidatePage(o.Node)
	case OpAppend:
		if len(o.Items) == 0 {
			return fmt.Errorf("render: op %d: append needs its items", o.Seq)
		}
	case OpMerge:
		if _, ok := o.Value.(map[string]any); !ok {
			if _, ok := o.Value.(Node); !ok {
				return fmt.Errorf("render: op %d: merge needs an object value", o.Seq)
			}
		}
	case OpFail:
		if o.Message == "" {
			return fmt.Errorf("render: op %d: fail needs its message", o.Seq)
		}
	}
	return nil
}

// Replace is the op that swaps the node id for node.
func Replace(seq int64, id string, node Node) Op {
	return Op{Seq: seq, Op: OpReplace, Target: id, Node: node}
}

// Append is the op that adds items to the data of the node id.
func Append(seq int64, id string, items ...any) Op {
	return Op{Seq: seq, Op: OpAppend, Target: id, Items: items}
}

// Fail is the op that shows an error card with message in place of node id.
func Fail(seq int64, id, message string) Op {
	return Op{Seq: seq, Op: OpFail, Target: id, Message: message}
}

// Complete is the op that ends a page's loading state.
func Complete(seq int64) Op { return Op{Seq: seq, Op: OpComplete} }

// Load is the method a section is loaded by: Method a method URI, Args its
// input. It answers an Answer (or a Def, or a page node).
type Load struct {
	Method string         `json:"method"`
	Args   map[string]any `json:"args,omitempty"`
}

// Placeholder is what a section shows until its answer: Kind skeleton, cards
// or rows, Count how many.
type Placeholder struct {
	Kind  string `json:"kind"`
	Count int    `json:"count,omitempty"`
}

// Section placeholders and when a section loads.
const (
	PlaceholderSkeleton = "skeleton"
	PlaceholderCards    = "cards"
	PlaceholderRows     = "rows"

	WhenVisible = "visible"
	WhenOpen    = "open"
	WhenEager   = "eager"
)

// AsSection makes any node a section: id, its stable node id; load, the
// method the host calls when the node comes into view (or its tab or
// accordion item opens); ph, what is drawn until the answer, which replaces
// the node by its id. Refine with Refresh and LoadWhen.
func (n Node) AsSection(id string, load Load, ph Placeholder) Node {
	return n.With("id", id).With("load", load).With("placeholder", ph)
}

// LoadedSection is a part of a page fed only by its own load:
// an empty box section - id, load, ph as AsSection's - that the host loads
// wherever it loads sections, progressive page or not. Its load args may read
// the page's selection (`${selection.<key>}`): it waits, showing empty, while
// a key they read is unset, and loads again when what they read changes.
func LoadedSection(id string, load Load, ph Placeholder, empty string) Node {
	n := Node{"kind": "box", "children": []Node{}}.AsSection(id, load, ph)
	if empty != "" {
		n["empty"] = empty
	}
	return n
}

// AsPushedSection makes any node a section its producer pushes
// : id, its stable node id; ph, what is drawn until the
// producer of the page panel replaces the node by its id (a replace op on
// the panel). It has no load: nobody calls for it.
func (n Node) AsPushedSection(id string, ph Placeholder) Node {
	return n.With("id", id).With("placeholder", ph)
}

// Refresh sets which changes re-read a section: the panel's refresh scope and
// object types (objectpane's PanelRefresh shape).
func (n Node) Refresh(scope string, types ...string) Node {
	r := map[string]any{"scope": scope}
	if len(types) > 0 {
		r["types"] = types
	}
	return n.With("refresh", r)
}

// LoadWhen sets when a section loads: visible (the default), open or eager.
func (n Node) LoadWhen(when string) Node { return n.With("when", when) }

// Pagination is the page envelope a paged load answers beside its items -
// the one shape display's paged nodes and crud's list read (crud's
// api.Pagination is this type): Total the post-filter count (0 when not
// reported), PageSize the items in this page, Offset of its first item,
// HasMore whether another page follows, NextCursor the opaque cursor for it.
type Pagination struct {
	Total      int64  `json:"total,omitempty"  desc:"Post-filter record count (0 when backend doesn't report)"`
	PageSize   int    `json:"pageSize"         desc:"Records in this page"`
	Offset     int    `json:"offset"           desc:"Offset of the first record in this page"`
	HasMore    bool   `json:"hasMore"          desc:"true = at least one more page is available"`
	NextCursor string `json:"nextCursor,omitempty" desc:"Opaque cursor for the next page (cursor-mode backends only)"`
}

// PageRequest is what a paged load receives: Limit items from Offset, or
// from Cursor when the producer pages by cursor.
type PageRequest struct {
	Limit  int    `json:"limit"`
	Offset int    `json:"offset,omitempty"`
	Cursor string `json:"cursor,omitempty"`
}

// PagedItems is what a paged load answers: a page of items and its envelope.
type PagedItems struct {
	Items []any      `json:"items"`
	Page  Pagination `json:"page"`
}
