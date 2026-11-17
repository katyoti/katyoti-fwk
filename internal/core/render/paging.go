// SPDX-License-Identifier: Apache-2.0

package render

import "strconv"

// PageOf answers one page of items for a paged load: the
// items from req.Offset (or from the offset req.Cursor names, as PageOf writes
// it), at most req.Limit of them (all when Limit is not positive), with the
// envelope a paged table reads - the total, the page's size and offset, whether
// another page follows and the cursor for it. A producer holding all its items
// answers its pages with it; one paging a store of its own answers PagedItems
// itself, in the same shape.
func PageOf(items []any, req PageRequest) PagedItems {
	off := req.Offset
	if req.Cursor != "" {
		if n, err := strconv.Atoi(req.Cursor); err == nil {
			off = n
		}
	}
	if off < 0 {
		off = 0
	}
	if off > len(items) {
		off = len(items)
	}
	end := len(items)
	if req.Limit > 0 && off+req.Limit < end {
		end = off + req.Limit
	}
	page := Pagination{Total: int64(len(items)), PageSize: end - off, Offset: off, HasMore: end < len(items)}
	if page.HasMore {
		page.NextCursor = strconv.Itoa(end)
	}
	out := make([]any, end-off)
	copy(out, items[off:end])
	return PagedItems{Items: out, Page: page}
}
