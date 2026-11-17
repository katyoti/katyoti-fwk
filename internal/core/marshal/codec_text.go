// SPDX-License-Identifier: Apache-2.0

package marshal

import "fmt"

// textCodec implements Codec for plain text.
// Unmarshal wraps the text in a map with a "content" key.
// Marshal extracts the "content" key from a map.
type textCodec struct{}

func (textCodec) Unmarshal(data []byte, target any) error {
	if m, ok := target.(*map[string]any); ok && m != nil {
		if *m == nil {
			*m = make(map[string]any)
		}
		(*m)["content"] = string(data)
		return nil
	}
	return fmt.Errorf("marshal: text format requires *map[string]any target")
}

func (textCodec) Marshal(data any) ([]byte, error) {
	if m, ok := data.(map[string]any); ok {
		if content, ok := m["content"].(string); ok {
			return []byte(content), nil
		}
	}
	return nil, fmt.Errorf("marshal: text format requires map with string 'content' key")
}
