// SPDX-License-Identifier: Apache-2.0

package contribapi

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
)

// derive_meta.go — the picker fields a kind declares as derived, read once
// from its data layer.
//
// A kind with `meta.from: data` hands its picker fields to the framework: the
// producer writes them once, in the data layer, under the kind's schema; the
// manifest builder fills meta from there (ApplyDerivedMeta); the census
// refuses an entry whose meta says otherwise (MetaDisagreements). One writer,
// so the name a listing shows and the name the body carries cannot drift —
// which is what two writers did (c22) and what putting the frontmatter into
// the schema would reopen without this.

// DeriveMeta reads `fields` (meta field names, see DerivableMetaFields) from
// the inline bytes of `layer` (JSON, or YAML when the layer's media type says
// so) and returns a ContributionMeta holding ONLY
// those fields. A layer without bytes is an error: derivation needs the
// content, and a kind that declares it must keep the data layer inline.
func DeriveMeta(c Contribution, layer string, fields []string) (ContributionMeta, error) {
	var out ContributionMeta
	raw := c.Layers[layer].Inline
	if len(raw) == 0 {
		return out, fmt.Errorf("%s: layer %q carries no bytes to derive meta from", c.ResolvedURI(), layer)
	}
	var data map[string]any
	if strings.Contains(c.Layers[layer].MediaType, "yaml") {
		// A layer kept as written (a data-type's DSL source): content, not
		// configuration, decoded with no variable expansion.
		if err := marshal.Unmarshal(raw, marshal.YAML, &data); err != nil {
			return out, fmt.Errorf("%s: layer %q is not a YAML mapping: %w", c.ResolvedURI(), layer, err)
		}
	} else if err := json.Unmarshal(raw, &data); err != nil {
		return out, fmt.Errorf("%s: layer %q is not a JSON object: %w", c.ResolvedURI(), layer, err)
	}
	for _, f := range fields {
		switch f {
		case "name":
			out.Name, _ = data["name"].(string)
		case "description":
			out.Description, _ = data["description"].(string)
		case "category":
			out.Category, _ = data["category"].(string)
		case "hashtags":
			if list, ok := data["hashtags"].([]any); ok {
				for _, v := range list {
					if s, ok := v.(string); ok {
						out.Hashtags = append(out.Hashtags, s)
					}
				}
			}
		case "icon":
			tabler, _ := data["iconName"].(string)
			emoji, _ := data["icon"].(string)
			out.Icon = NewIcon(tabler, emoji)
		case "advertised":
			if b, ok := data["advertised"].(bool); ok {
				out.Advertised = &b
			}
		default:
			return out, fmt.Errorf("%s: %q is not a derivable meta field", c.ResolvedURI(), f)
		}
	}
	return out, nil
}

// ApplyDerivedMeta fills the derived fields of c.Metadata from its data
// layer when c's kind declares `meta.from`; every other field of meta
// (MediaType, Annotations, and any field not listed) is left as the producer
// wrote it. A kind without `meta.from` is untouched.
func ApplyDerivedMeta(c *Contribution, kinds Kinds) error {
	layer, fields := kinds.MetaFrom(c.Type)
	return ApplyMetaFrom(c, layer, fields)
}

// ApplyMetaFrom is ApplyDerivedMeta for a reader that holds the kind's
// declaration itself (its `meta.from` layer and `meta.fields`) rather than a
// Kinds: the pack reader, which reads an entry by its kind's declaration even
// when the publishing binary does not declare that kind (a foreign kind its
// builder cannot derive). An empty layer derives nothing.
func ApplyMetaFrom(c *Contribution, layer string, fields []string) error {
	if layer == "" {
		return nil
	}
	derived, err := DeriveMeta(*c, layer, fields)
	if err != nil {
		return err
	}
	if c.Metadata == nil {
		c.Metadata = &ContributionMeta{}
	}
	for _, f := range fields {
		setMetaField(c.Metadata, derived, f)
	}
	return nil
}

// MetaDisagreements lists, for an entry whose kind derives its meta, every
// derived field where c.Metadata differs from what its data layer yields —
// empty when they agree or when the kind does not derive. The census reads
// this: a disagreement means a second writer.
func MetaDisagreements(c Contribution, kinds Kinds) []string {
	layer, fields := kinds.MetaFrom(c.Type)
	if layer == "" {
		return nil
	}
	derived, err := DeriveMeta(c, layer, fields)
	if err != nil {
		return []string{err.Error()}
	}
	have := ContributionMeta{}
	if c.Metadata != nil {
		have = *c.Metadata
	}
	var out []string
	for _, f := range fields {
		if !reflect.DeepEqual(metaField(have, f), metaField(derived, f)) {
			out = append(out, fmt.Sprintf("%s: meta.%s is %s, data derives %s", c.ResolvedURI(), f, show(metaField(have, f)), show(metaField(derived, f))))
		}
	}
	return out
}

func setMetaField(dst *ContributionMeta, src ContributionMeta, f string) {
	switch f {
	case "name":
		dst.Name = src.Name
	case "description":
		dst.Description = src.Description
	case "category":
		dst.Category = src.Category
	case "hashtags":
		dst.Hashtags = src.Hashtags
	case "icon":
		dst.Icon = src.Icon
	case "advertised":
		dst.Advertised = src.Advertised
	}
}

func metaField(m ContributionMeta, f string) any {
	switch f {
	case "name":
		return m.Name
	case "description":
		return m.Description
	case "category":
		return m.Category
	case "hashtags":
		if len(m.Hashtags) == 0 {
			return []string(nil)
		}
		return m.Hashtags
	case "icon":
		if m.Icon == nil {
			return (*ContributionIcon)(nil)
		}
		return m.Icon
	case "advertised":
		if m.Advertised == nil {
			return (*bool)(nil)
		}
		return m.Advertised
	}
	return nil
}

func show(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return strings.TrimSpace(string(b))
}
