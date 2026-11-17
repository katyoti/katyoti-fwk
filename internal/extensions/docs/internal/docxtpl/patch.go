// SPDX-License-Identifier: Apache-2.0

package docxtpl

import (
	"strings"

	"github.com/beevik/etree"
)

// patchDocument walks the XML tree and reassembles template expressions
// that Word fragmented across multiple <w:r> runs.
//
// Algorithm: for each run container (paragraph, sdtContent), collect all
// <w:t> text nodes in order, then use a state machine to detect template
// expressions that span multiple text nodes. When found, merge those
// text nodes into a single node with the complete expression.
func patchDocument(doc *etree.Document) {
	walkElements(doc.Root(), func(el *etree.Element) {
		switch {
		case el.Tag == "p" && el.Space == "w":
			patchRunContainer(el)
		case el.Tag == "sdtContent" && el.Space == "w":
			patchRunContainer(el)
		}
	})

	// Lift block tags: {%p → remove <w:p>, {%tr → remove <w:tr>, etc.
	liftBlockTags(doc.Root())

	// Remove showingPlcHdr from SDTs that contain template expressions.
	// This tells Word to display the actual text content instead of the placeholder.
	removeSDTPlaceholderFlags(doc.Root())
}

// patchRunContainer finds and merges template expressions that span
// multiple <w:t> text nodes within a container (paragraph or sdtContent).
func patchRunContainer(container *etree.Element) {
	// Remove proofErr elements first (spell-check markers between runs).
	removeProofErr(container)

	// Collect all text nodes in document order.
	textNodes := collectTextNodes(container)
	if len(textNodes) == 0 {
		return
	}

	// Quick check: any chance of template markers?
	fullText := concatText(textNodes)
	if !strings.Contains(fullText, "{") {
		return
	}

	// Run the expression scanner to find spans that need merging.
	spans := scanExpressions(textNodes)
	if len(spans) == 0 {
		return
	}

	// Merge each span: combine text into the first node, remove the rest.
	for i := len(spans) - 1; i >= 0; i-- {
		mergeSpan(spans[i])
	}
}

// textNode wraps an etree element (<w:t>) with its parent run (<w:r>).
type textNode struct {
	element *etree.Element // the <w:t> element
	run     *etree.Element // the parent <w:r> element
}

// expressionSpan identifies a contiguous range of text nodes that together
// form a complete template expression ({{...}}, {%...%}, or {#...#}).
type expressionSpan struct {
	nodes []textNode // the text nodes involved
	start int        // char offset in first node where expression starts
	end   int        // char offset in last node where expression ends
}

// scanState is the state machine for detecting template expressions.
type scanState int

const (
	stateIdle     scanState = iota // not in an expression
	stateSeenOpen                  // seen '{', waiting for second char
	stateInExpr                    // inside an expression, scanning for closer
)

// scanExpressions walks text nodes and finds template expressions that span
// multiple nodes. Returns spans that need merging.
//
// State machine:
//
//	IDLE → '{' → SEEN_OPEN
//	SEEN_OPEN → '{' → IN_EXPR (closer = "}}")
//	SEEN_OPEN → '%' → IN_EXPR (closer = "%}")
//	SEEN_OPEN → '#' → IN_EXPR (closer = "#}")
//	SEEN_OPEN → other → IDLE (false alarm)
//	IN_EXPR → closer[0] seen, then closer[1] → IDLE (expression complete)
//
// Only spans crossing multiple text nodes are returned (single-node expressions
// don't need merging).
func scanExpressions(nodes []textNode) []expressionSpan {
	var spans []expressionSpan

	state := stateIdle
	var closer [2]byte    // the 2-char closing sequence (e.g., '}', '}')
	var seenCloser1 bool  // seen first char of closer
	var exprStartNode int // index of the node where the expression started
	var exprStartChar int // char offset in that node

	for nodeIdx, tn := range nodes {
		text := tn.element.Text()
		for charIdx := 0; charIdx < len(text); charIdx++ {
			ch := text[charIdx]

			switch state {
			case stateIdle:
				if ch == '{' {
					state = stateSeenOpen
					exprStartNode = nodeIdx
					exprStartChar = charIdx
				}

			case stateSeenOpen:
				switch ch {
				case '{':
					state = stateInExpr
					closer = [2]byte{'}', '}'}
					seenCloser1 = false
				case '%':
					state = stateInExpr
					closer = [2]byte{'%', '}'}
					seenCloser1 = false
				case '#':
					state = stateInExpr
					closer = [2]byte{'#', '}'}
					seenCloser1 = false
				default:
					// False alarm — '{' followed by something else.
					state = stateIdle
				}

			case stateInExpr:
				if seenCloser1 {
					if ch == closer[1] {
						// Expression complete!
						endNode := nodeIdx
						endChar := charIdx + 1 // exclusive

						// Only record if expression spans multiple nodes.
						if endNode > exprStartNode {
							spans = append(spans, expressionSpan{
								nodes: nodes[exprStartNode : endNode+1],
								start: exprStartChar,
								end:   endChar,
							})
						}
						state = stateIdle
					} else {
						// False alarm on closer[0] — keep scanning.
						seenCloser1 = false
						// Re-check current char as potential closer[0].
						if ch == closer[0] {
							seenCloser1 = true
						}
					}
				} else if ch == closer[0] {
					seenCloser1 = true
				}
			}
		}

		// If we're in SEEN_OPEN state at end of a text node, that's fine —
		// the '{' was at the end and the qualifier might be in the next node.
		// If we're in IN_EXPR at end of a node, continue scanning next node.
	}

	return spans
}

// mergeSpan combines the text from multiple text nodes into the first one
// and removes the now-empty runs.
func mergeSpan(span expressionSpan) {
	if len(span.nodes) < 2 {
		return
	}

	// Build the merged text from all nodes in the span.
	var merged strings.Builder
	for i, tn := range span.nodes {
		text := tn.element.Text()
		if i == 0 {
			// First node: include text from start offset.
			merged.WriteString(text[:span.start])
		}
		if i == 0 {
			merged.WriteString(text[span.start:])
		} else if i == len(span.nodes)-1 {
			merged.WriteString(text[:span.end])
		} else {
			merged.WriteString(text)
		}
	}

	// Set the first node's text to everything before the expression + the merged expression.
	firstText := span.nodes[0].element.Text()
	lastText := span.nodes[len(span.nodes)-1].element.Text()

	newFirst := firstText[:span.start] + merged.String()[span.start:]
	// If there's text after the expression in the last node, keep it.
	trailing := ""
	if span.end < len(lastText) {
		trailing = lastText[span.end:]
	}

	span.nodes[0].element.SetText(newFirst + trailing)
	span.nodes[0].element.CreateAttr("xml:space", "preserve")

	// Remove the other runs (if they're now empty or fully consumed).
	for i := 1; i < len(span.nodes); i++ {
		tn := span.nodes[i]
		if i == len(span.nodes)-1 && span.end < len(tn.element.Text()) {
			// Last node has trailing text — keep the run but clear consumed part.
			// Already handled by appending trailing to first node.
		}
		// Remove this run from its parent.
		if tn.run != nil && tn.run.Parent() != nil {
			tn.run.Parent().RemoveChild(tn.run)
		}
	}
}

// --- Helper functions ---

// collectTextNodes returns all <w:t> elements (with their parent <w:r>)
// in document order within a container.
func collectTextNodes(container *etree.Element) []textNode {
	var nodes []textNode
	for _, child := range container.ChildElements() {
		if child.Tag == "r" && child.Space == "w" {
			for _, t := range child.ChildElements() {
				if t.Tag == "t" && t.Space == "w" {
					nodes = append(nodes, textNode{element: t, run: child})
				}
			}
		}
	}
	return nodes
}

func concatText(nodes []textNode) string {
	var sb strings.Builder
	for _, tn := range nodes {
		sb.WriteString(tn.element.Text())
	}
	return sb.String()
}

func removeProofErr(container *etree.Element) {
	var toRemove []*etree.Element
	for _, child := range container.ChildElements() {
		if child.Tag == "proofErr" {
			toRemove = append(toRemove, child)
		}
	}
	for _, el := range toRemove {
		container.RemoveChild(el)
	}
}

// liftBlockTags handles {%p for %}, {%tr for %}, {%tc for %}, {%r for %}.
// The prefix (p, tr, tc, r) indicates that the ENTIRE containing XML element
// should be removed, leaving only the Jinja2 tag as raw text.
//
// Example: <w:p>...<w:t>{%p for x in items %}</w:t>...</w:p>
// becomes: {% for x in items %}
// (the entire <w:p> is replaced with just the tag text)
//
// This is the Python docxtpl approach (template.py lines 176-185).
func liftBlockTags(root *etree.Element) {
	// Map of prefix → containing element tag to remove
	prefixes := map[string]string{
		"p":  "p",
		"tr": "tr",
		"tc": "tc",
		"r":  "r",
	}

	for prefix, containerTag := range prefixes {
		liftPrefix(root, prefix, containerTag)
	}
}

func liftPrefix(root *etree.Element, prefix, containerTag string) {
	// We need multiple passes because removing elements changes the tree.
	for {
		found := false
		walkElements(root, func(el *etree.Element) {
			if found {
				return
			}
			if el.Tag != "t" || el.Space != "w" {
				return
			}
			text := el.Text()
			// Check for {%PREFIX  or {%-PREFIX
			marker1 := "{%" + prefix + " "
			marker2 := "{%-" + prefix + " "
			if !strings.Contains(text, marker1) && !strings.Contains(text, marker2) {
				return
			}

			// Extract the jinja tag: strip the prefix
			cleaned := text
			cleaned = strings.ReplaceAll(cleaned, marker1, "{% ")
			cleaned = strings.ReplaceAll(cleaned, marker2, "{%- ")

			// Find the containing element (w:p, w:tr, etc.) and replace it
			// with a raw text node containing just the jinja tag.
			container := findAncestor(el, containerTag)
			if container == nil || container.Parent() == nil {
				// No matching container — just strip the prefix in place.
				el.SetText(cleaned)
				return
			}

			parent := container.Parent()
			// Insert the raw text as a comment element that gonja will process.
			// We use a special <w:r><w:t> wrapper so it survives in the XML.
			placeholder := etree.NewElement("w:jinja")
			placeholder.SetText(cleaned)

			// Replace the container with the placeholder.
			idx := childIndex(parent, container)
			parent.RemoveChild(container)
			parent.InsertChildAt(idx, placeholder)
			found = true
		})
		if !found {
			break
		}
	}
}

func findAncestor(el *etree.Element, tag string) *etree.Element {
	current := el.Parent()
	for current != nil {
		if current.Tag == tag && current.Space == "w" {
			return current
		}
		current = current.Parent()
	}
	return nil
}

func childIndex(parent, child *etree.Element) int {
	for i, c := range parent.Child {
		if c == child {
			return i
		}
	}
	return len(parent.Child)
}

// removeSDTPlaceholderFlags removes <w:showingPlcHdr/> from SDTs
// that had their content replaced by template expressions.
// Without this, Word displays the SDT's placeholder text instead of the actual value.
func removeSDTPlaceholderFlags(root *etree.Element) {
	walkElements(root, func(el *etree.Element) {
		if el.Tag != "sdtPr" || el.Space != "w" {
			return
		}
		// Check if sibling sdtContent has template-rendered text
		parent := el.Parent()
		if parent == nil {
			return
		}
		for _, child := range parent.ChildElements() {
			if child.Tag == "sdtContent" && child.Space == "w" {
				// Collect text from sdtContent
				var text strings.Builder
				walkElements(child, func(inner *etree.Element) {
					if inner.Tag == "t" {
						text.WriteString(inner.Text())
					}
				})
				// If text is non-empty (template was rendered), remove placeholder flag
				if text.Len() > 0 {
					for _, prop := range el.ChildElements() {
						if prop.Tag == "showingPlcHdr" {
							el.RemoveChild(prop)
						}
					}
				}
			}
		}
	})
}

func walkElements(el *etree.Element, fn func(*etree.Element)) {
	if el == nil {
		return
	}
	fn(el)
	for _, child := range el.ChildElements() {
		walkElements(child, fn)
	}
}
