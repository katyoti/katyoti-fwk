// SPDX-License-Identifier: Apache-2.0

package marshal

import (
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// frontmatterCodec implements Codec for Markdown with YAML front matter.
type frontmatterCodec struct{}

func (frontmatterCodec) Unmarshal(data []byte, target any) error {
	return unmarshalFrontmatter(data, target)
}

func (frontmatterCodec) Marshal(data any) ([]byte, error) {
	return marshalFrontmatter(data)
}

// --- helpers ---

var fmBoundary = regexp.MustCompile(`(?m)^-{3,}\s*$`)

// splitFrontmatter splits text into YAML front matter and body content.
// Returns ("", text) if no valid front matter is found.
func splitFrontmatter(text string) (string, string) {
	if !fmBoundary.MatchString(text) {
		return "", text
	}
	parts := fmBoundary.Split(text, 3)
	// Valid frontmatter requires --- at the very start (first part is empty/whitespace).
	if len(parts) < 3 || strings.TrimSpace(parts[0]) != "" {
		return "", text
	}
	return parts[1], parts[2]
}

// unmarshalFrontmatter parses a Markdown document with YAML front matter.
// The front matter fields are unmarshalled into target, and a "content" key
// is added with the body text (when target is a map or has a Content field).
func unmarshalFrontmatter(data []byte, target any) error {
	fm, body := splitFrontmatter(string(data))

	// First unmarshal the YAML front matter.
	if fm != "" {
		if err := yaml.Unmarshal([]byte(fm), target); err != nil {
			return fmt.Errorf("frontmatter yaml: %w", err)
		}
	}

	// Try to set "content" on the target if it's a map.
	if m, ok := target.(*map[string]any); ok && m != nil {
		if *m == nil {
			*m = make(map[string]any)
		}
		(*m)["content"] = body
	}
	return nil
}

// marshalFrontmatter serializes data as YAML front matter + body content.
// If data is a map with a "content" key, that becomes the body.
func marshalFrontmatter(data any) ([]byte, error) {
	m, ok := data.(map[string]any)
	if !ok {
		// Fall back to pure YAML.
		yamlBytes, err := yaml.Marshal(data)
		if err != nil {
			return nil, err
		}
		return []byte("---\n" + string(yamlBytes) + "---\n"), nil
	}

	// Extract content field for body.
	body := ""
	header := make(map[string]any, len(m))
	for k, v := range m {
		if k == "content" {
			if s, ok := v.(string); ok {
				body = s
			}
		} else {
			header[k] = v
		}
	}

	yamlBytes, err := yaml.Marshal(header)
	if err != nil {
		return nil, err
	}

	var sb strings.Builder
	sb.WriteString("---\n")
	sb.Write(yamlBytes)
	sb.WriteString("---\n")
	if body != "" {
		sb.WriteString(body)
	}
	return []byte(sb.String()), nil
}
