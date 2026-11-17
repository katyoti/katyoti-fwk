// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"strings"
	"testing"
)

func TestParse_FilterField(t *testing.T) {
	data := []byte(`---
name: review-active-document
type: agent
kind: object
types: [document]
filter: |
  object.content.status == "active"
---
body`)
	got, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !strings.Contains(got.Filter, `object.content.status == "active"`) {
		t.Errorf("Filter = %q, want to contain the expression", got.Filter)
	}
}

func TestParse_FilterCompileError(t *testing.T) {
	// Malformed expression — discovery surfaces the parse error early.
	data := []byte(`---
name: bad-filter
type: agent
kind: object
types: [document]
filter: 'object. == "x"'
---
body`)
	_, err := Parse(data)
	if err == nil {
		t.Fatal("expected compile error for malformed filter")
	}
	if !strings.Contains(err.Error(), "filter compile error") {
		t.Errorf("error should mention filter compile; got: %v", err)
	}
}

func TestParse_FilterEmpty(t *testing.T) {
	// Filter is optional; absence keeps the task valid.
	data := []byte(`---
name: no-filter
type: agent
kind: object
types: [document]
---
body`)
	got, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got.Filter != "" {
		t.Errorf("Filter = %q, want empty", got.Filter)
	}
}

func TestParse_FilterNotInConfig(t *testing.T) {
	// `filter` is meta — must not leak into Config.
	data := []byte(`---
name: doc-review
type: agent
kind: object
types: [document]
filter: object.content.status == "active"
capability: smart-best
---
body`)
	got, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if _, present := got.Config["filter"]; present {
		t.Error("filter leaked into Config — must be a meta field")
	}
}

// TestParse_FilterInTheRetiredVocabularyIsRefused — a filter naming `node` is
// refused at parse, naming the task and the name. Before p7o it loaded, and
// CWM then disabled its action on every object with a reason nobody saw.
func TestParse_FilterInTheRetiredVocabularyIsRefused(t *testing.T) {
	data := []byte(`---
name: old-vocabulary
type: agent
kind: object
types: [document]
filter: node.state == "active"
---
body`)
	_, err := Parse(data)
	if err == nil {
		t.Fatal("a filter in the node vocabulary must be refused at parse")
	}
	if !strings.Contains(err.Error(), "old-vocabulary") || !strings.Contains(err.Error(), "node") {
		t.Errorf("the refusal must name the task and the unknown name; got: %v", err)
	}
}
