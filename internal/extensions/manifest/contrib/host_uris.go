// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"encoding/json"
	"strings"

	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// ResolveHostURIs gives the host's name to every host-relative URI in a
// published declaration: a JSON string value that parses as a URI (resource,
// tool or event) with the empty provider becomes `<provider>` + the string.
// The rewrite is in place — every other byte is kept, so a declaration that
// names no host-relative URI publishes exactly the bytes it was marshalled to.
func ResolveHostURIs(provider string, body []byte) []byte {
	if provider == "" {
		return body
	}
	var out []byte // nil until the first rewrite
	last := 0
	for i := 0; i < len(body); i++ {
		if body[i] != '"' {
			continue
		}
		end := stringEnd(body, i)
		if end < 0 {
			return body // not JSON: publish as is
		}
		start, lit := i, body[i:end+1]
		i = end
		if isKey(body, end+1) {
			continue
		}
		var v string
		if json.Unmarshal(lit, &v) != nil || !strings.HasPrefix(v, ":") {
			continue
		}
		if _, err := mbapi.ParseURI(v); err != nil {
			continue
		}
		resolved, err := json.Marshal(provider + v)
		if err != nil {
			continue
		}
		out = append(out, body[last:start]...)
		out = append(out, resolved...)
		last = end + 1
	}
	if out == nil {
		return body
	}
	return append(out, body[last:]...)
}

// stringEnd returns the index of the quote closing the JSON string opened at
// start, or -1.
func stringEnd(b []byte, start int) int {
	for j := start + 1; j < len(b); j++ {
		switch b[j] {
		case '\\':
			j++
		case '"':
			return j
		}
	}
	return -1
}

// isKey reports whether the string ending before i is an object key — the
// next non-space byte is ':'.
func isKey(b []byte, i int) bool {
	for ; i < len(b); i++ {
		switch b[i] {
		case ' ', '\t', '\n', '\r':
			continue
		case ':':
			return true
		}
		return false
	}
	return false
}
