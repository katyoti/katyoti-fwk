// SPDX-License-Identifier: Apache-2.0

package local

import (
	"encoding/json"

	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
)

// mapKTSPInfo translates a ktsp.ContainerInfo (the runtime introspection
// surface) into the manifest's descriptor surface — `[]mbapi.SlotType`
// + `[]mbapi.Slot`. Both the SlotTypes and Slots collections are
// re-ordered for hash determinism: SlotTypes by Type, Slots by Name.
//
// The mapping deliberately drops `Description` from the slot-type
// contract because descriptions are derived from per-method `MethodInfo`
// and don't add information at the slot-type level. Tags ride through.
//
// JSON Schemas (`InputSchema` / `OutputSchema`) are stored in ktsp as
// the decoded JSON tree (`any`); the manifest stores them as
// `json.RawMessage` to avoid coupling consumers to the schema library.
// We marshal-and-store rather than re-emit the decoded tree so the
// hash stays content-addressed across encoders.
func mapKTSPInfo(info ktsp.ContainerInfo) ([]mbapi.SlotType, []mbapi.Slot) {
	slotTypes := make([]mbapi.SlotType, 0, len(info.SlotTypes))
	for _, st := range info.SlotTypes {
		slotTypes = append(slotTypes, mbapi.SlotType{
			Type:    string(st.Type),
			Version: st.Version,
			Methods: mapMethods(st.Methods),
		})
	}

	slots := make([]mbapi.Slot, 0, len(info.Slots))
	for _, s := range info.Slots {
		slots = append(slots, mbapi.Slot{
			Name:        s.Name,
			Type:        string(s.SlotType),
			Description: s.Description,
		})
	}

	return slotTypes, slots
}

// mapMethods converts ktsp.MethodInfo into mbapi.SlotMethod, encoding
// the schemas as canonical JSON (raw message). Methods with nil schemas
// are emitted with the field omitted (`omitempty`).
func mapMethods(in []ktsp.MethodInfo) []mbapi.SlotMethod {
	out := make([]mbapi.SlotMethod, 0, len(in))
	for _, m := range in {
		out = append(out, mbapi.SlotMethod{
			Name:         m.Name,
			Description:  m.Description,
			InputSchema:  marshalSchemaOrNil(m.InputSchema),
			OutputSchema: marshalSchemaOrNil(m.OutputSchema),
			Tags:         tagsToSlice(m.Tags),
			TimeoutS:     m.TimeoutS,
		})
	}
	return out
}

func marshalSchemaOrNil(schema any) json.RawMessage {
	if schema == nil {
		return nil
	}
	b, err := json.Marshal(schema)
	if err != nil {
		// Skip the schema rather than fail the whole descriptor build.
		// Caller's hash will still be deterministic across runs because
		// the same schema marshals the same way (or fails the same way).
		return nil
	}
	return b
}

// tagsToSlice flattens ktsp.MethodTags (a `map[string]string`) into a
// stable `[]string` for the manifest's `tags` field. Today's manifest
// surface uses `[]string` (e.g. ["cli", "mcp", "internal"]); legacy
// key=value tags get joined with `=`. Sorted for hash determinism.
func tagsToSlice(tags ktsp.MethodTags) []string {
	if len(tags) == 0 {
		return nil
	}
	out := make([]string, 0, len(tags))
	for k, v := range tags {
		if v == "" || v == "true" {
			out = append(out, k)
		} else {
			out = append(out, k+"="+v)
		}
	}
	// stable order for determinism — Go map iteration is randomised.
	sortStrings(out)
	return out
}

// sortStrings is an inline, dependency-free string sort to keep this
// file's import surface minimal. Insertion-sort is fine — tag counts
// are tiny.
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
