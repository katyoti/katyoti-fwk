// SPDX-License-Identifier: Apache-2.0

package render

import (
	"encoding/json"
	"testing"
)

// TestPageOfAnswersOnePageAndWhereTheNextStarts: a paged load answers a page
// of its items with the envelope a paged table reads.
func TestPageOfAnswersOnePageAndWhereTheNextStarts(t *testing.T) {
	items := make([]any, 45)
	for i := range items {
		items[i] = i
	}
	p := PageOf(items, PageRequest{Limit: 20})
	if len(p.Items) != 20 || p.Items[0] != 0 || p.Page.Total != 45 || !p.Page.HasMore || p.Page.NextCursor != "20" || p.Page.Offset != 0 {
		t.Fatalf("first page: %+v", p.Page)
	}
	p = PageOf(items, PageRequest{Limit: 20, Cursor: p.Page.NextCursor})
	if p.Items[0] != 20 || p.Page.Offset != 20 || !p.Page.HasMore {
		t.Fatalf("second page by cursor: %+v", p.Page)
	}
	p = PageOf(items, PageRequest{Limit: 20, Offset: 40})
	if len(p.Items) != 5 || p.Page.HasMore || p.Page.NextCursor != "" || p.Page.PageSize != 5 {
		t.Fatalf("last page: %+v", p.Page)
	}
	if p := PageOf(items, PageRequest{Offset: 99}); len(p.Items) != 0 || p.Page.HasMore {
		t.Fatalf("past the end: %+v", p)
	}
	if p := PageOf(items, PageRequest{}); len(p.Items) != 45 || p.Page.HasMore {
		t.Fatalf("no limit answers all: %+v", p.Page)
	}
	raw, _ := json.Marshal(PageOf(items, PageRequest{Limit: 1}))
	var back struct {
		Items []any `json:"items"`
		Page  struct {
			HasMore    bool   `json:"hasMore"`
			NextCursor string `json:"nextCursor"`
		} `json:"page"`
	}
	if json.Unmarshal(raw, &back) != nil || !back.Page.HasMore || back.Page.NextCursor != "1" || len(back.Items) != 1 {
		t.Fatalf("the wire shape the table reads: %s", raw)
	}
}
