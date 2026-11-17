// SPDX-License-Identifier: Apache-2.0

package web_search

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSearchInput_EmptyQuery(t *testing.T) {
	_, err := executeWebSearch(nil, SearchInput{Query: ""})
	if err == nil || !strings.Contains(err.Error(), "query is required") {
		t.Errorf("err = %v, want 'query is required'", err)
	}
}

func TestDescribe_Normal(t *testing.T) {
	raw, _ := json.Marshal(SearchInput{Query: "golang testing"})
	got := describeWebSearch(raw)
	if got != `web_search query="golang testing"` {
		t.Errorf("got %q", got)
	}
}

func TestDescribe_LongQuery(t *testing.T) {
	long := strings.Repeat("x", 100)
	raw, _ := json.Marshal(SearchInput{Query: long})
	got := describeWebSearch(raw)
	if !strings.Contains(got, "...") {
		t.Error("should truncate long queries")
	}
}

func TestDescribe_Invalid(t *testing.T) {
	got := describeWebSearch(json.RawMessage(`invalid`))
	if got != "web_search <invalid>" {
		t.Errorf("got %q", got)
	}
}
