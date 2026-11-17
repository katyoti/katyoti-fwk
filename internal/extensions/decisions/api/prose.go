// SPDX-License-Identifier: Apache-2.0

package api

import (
	"regexp"
	"strings"
)

// OptionsHeading opens a decision's prose section holding one `### <label>`
// section per option: what it means, its pros, its cons, its cost, what
// follows if it is chosen. Prepare writes it; every view of a decision reads
// it back through ReadPrepared.
const OptionsHeading = "## Options"

// DecisionHeading opens the prose saying why a decided decision chose what it
// chose.
const DecisionHeading = "## Decision"

// The keys of an option's prose section, as Prepare writes them and
// ReadPrepared reads them back. They are markup, not text for the person: a
// view words them in the person's language.
const (
	KeyMeans    = "Means"
	KeyPros     = "Pros"
	KeyCons     = "Cons"
	KeyCost     = "Cost"
	KeyIfChosen = "If chosen"
)

// PreparedOption is one option as its prose section says it: the keys read
// into its detail, any other prose kept as Text. Found is false for an
// option with no section (its label alone).
type PreparedOption struct {
	OptionDetail
	// Text is the section's prose that holds no key (written by hand): drawn
	// as it is.
	Text  string `json:"text,omitempty"`
	Found bool   `json:"found,omitempty"`
}

// Prepared is what a decision's body gives the person who decides: the
// reasoning (the question and its context) and one option per label, in the
// labels' order.
type Prepared struct {
	// Reasoning is the body without the options' section when an option has
	// one (the cards draw it) and without the `## Decision` prose.
	Reasoning string           `json:"reasoning,omitempty"`
	Options   []PreparedOption `json:"options,omitempty"`
}

// Any reports whether at least one option has its prose section.
func (p Prepared) Any() bool {
	for _, o := range p.Options {
		if o.Found {
			return true
		}
	}
	return false
}

// keyLine is a key line of an option's section: `- **Key:** value`.
var keyLine = regexp.MustCompile(`^\s*-\s+\*\*([^*:]+):\*\*\s*(.*)$`)

// subItem is an item of a key's list: an indented `- value`.
var subItem = regexp.MustCompile(`^\s+[-*]\s+(.*)$`)

// ReadPrepared reads a decision's body for its options' labels.
func ReadPrepared(body string, labels []string) Prepared {
	sections := map[string]string{}
	lines := strings.Split(body, "\n")
	for i := 0; i < len(lines); i++ {
		if !strings.HasPrefix(lines[i], "### ") {
			continue
		}
		label := strings.TrimSpace(strings.TrimPrefix(lines[i], "### "))
		var sec []string
		j := i + 1
		for ; j < len(lines) && !strings.HasPrefix(lines[j], "## ") && !strings.HasPrefix(lines[j], "### "); j++ {
			sec = append(sec, lines[j])
		}
		if _, seen := sections[label]; !seen {
			sections[label] = strings.Join(sec, "\n")
		}
		i = j - 1
	}
	p := Prepared{Options: make([]PreparedOption, 0, len(labels))}
	for _, label := range labels {
		o := PreparedOption{OptionDetail: OptionDetail{Label: label}}
		if sec, ok := sections[label]; ok {
			o = parseOption(label, sec)
		}
		p.Options = append(p.Options, o)
	}
	reasoning := withoutSection(body, DecisionHeading)
	if p.Any() {
		reasoning = withoutSection(reasoning, OptionsHeading)
	}
	p.Reasoning = strings.TrimSpace(reasoning)
	return p
}

func parseOption(label, sec string) PreparedOption {
	o := PreparedOption{OptionDetail: OptionDetail{Label: label}, Found: true}
	var key string
	var other []string
	keyed := false
	for _, l := range strings.Split(sec, "\n") {
		if m := keyLine.FindStringSubmatch(l); m != nil && !strings.HasPrefix(l, " ") {
			key, keyed = strings.TrimSpace(m[1]), true
			v := strings.TrimSpace(m[2])
			switch key {
			case KeyMeans:
				o.Means = v
			case KeyCost:
				o.Cost = v
			case KeyIfChosen:
				o.Consequence = v
			case KeyPros:
				if v != "" {
					o.Pros = append(o.Pros, v)
				}
			case KeyCons:
				if v != "" {
					o.Cons = append(o.Cons, v)
				}
			default:
				key = ""
				other = append(other, l)
			}
			continue
		}
		if m := subItem.FindStringSubmatch(l); m != nil && (key == KeyPros || key == KeyCons) {
			if key == KeyPros {
				o.Pros = append(o.Pros, strings.TrimSpace(m[1]))
			} else {
				o.Cons = append(o.Cons, strings.TrimSpace(m[1]))
			}
			continue
		}
		key = ""
		other = append(other, l)
	}
	if rest := strings.TrimSpace(strings.Join(other, "\n")); rest != "" {
		o.Text = rest
	}
	if !keyed {
		o.Text = strings.TrimSpace(sec)
	}
	return o
}

// withoutSection is the body without its level-2 section headed heading (up
// to the next level-2 heading).
func withoutSection(body, heading string) string {
	lines := strings.Split(body, "\n")
	out := make([]string, 0, len(lines))
	skip := false
	for _, l := range lines {
		if strings.HasPrefix(l, "## ") {
			skip = strings.TrimSpace(l) == heading
		}
		if !skip {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}
