// SPDX-License-Identifier: Apache-2.0

package contribapi

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// format.go — how a kind's format evolves between versions.
//
// A kind's FORMAT is the major.minor of the data-type its declaration names
// (`schema:`), additive within a major — the rule KTSP's NegotiateProtocol
// applies to the wire protocol. A binary states the formats it writes in its
// sidecar-info (`formats: {kind: M.m}`, derived by the manifest builder from
// what it contributes, never authored); a host reading a member's entries of a
// kind compares the member's format with its own:
//
//   - same or older: strict — a key no field takes is a typo, refused;
//   - newer minor: the member added keys this host does not know — they are
//     ignored and the entry is read, with a finding naming them;
//   - newer major: the member writes a format this host cannot read — that
//     kind's entries from that member are refused, saying to update the host.
//
// There is no `x-` namespace: a new key is a new minor of the data-type.

// SidecarInfoKind is the kind every binary publishes its identity as — and,
// since sidecar-info 1.1, the formats it writes.
const SidecarInfoKind = "sidecar-info"

// Format is a kind's major.minor.
type Format struct {
	Major, Minor int
}

// ParseFormat reads "M.m" or "M.m.p" (a data-type version); false when it is
// neither.
func ParseFormat(v string) (Format, bool) {
	parts := strings.Split(strings.TrimPrefix(strings.TrimSpace(v), "v"), ".")
	if len(parts) < 2 || len(parts) > 3 {
		return Format{}, false
	}
	maj, err1 := strconv.Atoi(parts[0])
	min, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || maj < 0 || min < 0 {
		return Format{}, false
	}
	return Format{Major: maj, Minor: min}, true
}

// String is "M.m".
func (f Format) String() string { return fmt.Sprintf("%d.%d", f.Major, f.Minor) }

// FormatVerdict is how a host reads a member's entries of one kind.
type FormatVerdict int

const (
	// FormatStrict: the member writes this host's format or an older one —
	// every key must be one a field takes.
	FormatStrict FormatVerdict = iota
	// FormatNewerMinor: the member writes a newer minor — keys this host does
	// not know are ignored, with a finding.
	FormatNewerMinor
	// FormatNewerMajor: the member writes a newer major — its entries of the
	// kind are refused.
	FormatNewerMajor
)

// JudgeFormat compares the format a member writes with the one the host reads.
// An unknown side (empty or unreadable) is strict: a member that states no
// format is read as this host's own.
func JudgeFormat(host, member string) FormatVerdict {
	h, ok1 := ParseFormat(host)
	m, ok2 := ParseFormat(member)
	if !ok1 || !ok2 {
		return FormatStrict
	}
	switch {
	case m.Major > h.Major:
		return FormatNewerMajor
	case m.Major == h.Major && m.Minor > h.Minor:
		return FormatNewerMinor
	default:
		return FormatStrict
	}
}

// NewerMajorMessage is the refusal of a member's entries of a kind whose
// format is a newer major than the host's.
func NewerMajorMessage(member, kind, memberFormat, hostFormat string) string {
	return fmt.Sprintf("%s writes %s in format %s; this hub reads %s: update the hub to read its %s entries",
		member, kind, memberFormat, hostFormat, kind)
}

// NewerMinorMessage is the finding on an entry read in a newer minor: the keys
// this host ignored.
func NewerMinorMessage(member, kind, memberFormat, hostFormat string, ignored []string) string {
	return fmt.Sprintf("%s writes %s in format %s, newer than this hub's %s: %s ignored (update the hub to read them)",
		member, kind, memberFormat, hostFormat, strings.Join(ignored, ", "))
}

// DataTypeFormat reads a data-type DSL's `version:` as a format; "" when the
// DSL states none or it is not a version.
func DataTypeFormat(dsl []byte) string {
	var head struct {
		Version string `yaml:"version" json:"version"`
	}
	if err := yaml.Unmarshal(dsl, &head); err != nil {
		return ""
	}
	f, ok := ParseFormat(head.Version)
	if !ok {
		return ""
	}
	return f.String()
}

// DeriveFormats is the format of every kind the non-system entries of cs are
// of, read from the kind's declaration and the data-type it names — both among
// cs (a binary carries the system set and its own). A kind whose declaration or
// data-type is not in cs, or whose data-type states no version, has none.
func DeriveFormats(cs []Contribution) map[string]string {
	kinds := KindsFrom(cs)
	versions := map[string]string{}
	for i := range cs {
		if cs[i].Type != ResourceTypeDataType {
			continue
		}
		if f := DataTypeFormat(cs[i].Source()); f != "" {
			versions[cs[i].ResolvedURI()] = f
		}
	}
	out := map[string]string{}
	for i := range cs {
		c := cs[i]
		if c.Provider == SystemProvider || out[c.Type] != "" {
			continue
		}
		if f := versions[kinds.SchemaFor(c.Type)]; f != "" {
			out[c.Type] = f
		}
	}
	return out
}

// SetFormats writes formats into a sidecar-info entry's data layer (`formats`),
// keeping every other field. A nil or empty map removes nothing and adds
// nothing.
func SetFormats(c *Contribution, formats map[string]string) error {
	if c.Type != SidecarInfoKind || len(formats) == 0 {
		return nil
	}
	var body map[string]any
	if d := c.Data(); len(d) > 0 {
		if err := json.Unmarshal(d, &body); err != nil {
			return fmt.Errorf("sidecar-info %s: %w", c.ResolvedURI(), err)
		}
	}
	if body == nil {
		body = map[string]any{}
	}
	body["formats"] = formats
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	c.SetData(b)
	return nil
}

// MemberFormats is the formats a member's sidecar-info states, from its
// manifest's entries; empty when it states none (a member built before
// sidecar-info 1.1 is read as writing the host's formats).
func MemberFormats(cs []Contribution) map[string]string {
	for i := range cs {
		if cs[i].Type != SidecarInfoKind {
			continue
		}
		var body struct {
			Formats map[string]string `json:"formats"`
		}
		if json.Unmarshal(cs[i].Data(), &body) == nil && len(body.Formats) > 0 {
			return body.Formats
		}
	}
	return nil
}

// FormatFindings compares every format a member states with the host's: one
// line per kind the host cannot read in full — a newer major (refused) or a
// newer minor (keys ignored). Sorted by kind. Used before install (marketplace)
// and by a universe lint.
func FormatFindings(member string, host, formats map[string]string) (refused, tolerated []string) {
	kinds := make([]string, 0, len(formats))
	for k := range formats {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	for _, k := range kinds {
		h, ok := host[k]
		if !ok {
			continue // a kind this host does not own: its declarer reads it
		}
		switch JudgeFormat(h, formats[k]) {
		case FormatNewerMajor:
			refused = append(refused, NewerMajorMessage(member, k, formats[k], h))
		case FormatNewerMinor:
			tolerated = append(tolerated, fmt.Sprintf("%s writes %s in format %s, newer than this hub's %s: keys this hub does not know are ignored",
				member, k, formats[k], h))
		}
	}
	return refused, tolerated
}
