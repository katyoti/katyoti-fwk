// SPDX-License-Identifier: Apache-2.0

package ktsp

import (
	"encoding/json"
)

// GenerateManifest produces a JSON-serializable manifest of a container's
// public slots and method contracts. Internal slots (internal="true") are
// filtered out.
//
// Wire shape: SlotTypeContract is the **single source of truth** for method
// definitions (name, description, tags, InputSchema, OutputSchema). Each
// Slot entry references its type via SlotType and MUST NOT carry its own
// Methods copy — consumers (including the sidecar plugin bridge) resolve a
// slot's callable surface by looking up its contract in SlotTypes.
//
// Untyped slots (SlotType == "") still emit their Methods inline because
// there is no contract to reference; this is a legacy escape hatch.
//
// Used by sidecar `ktsp describe` commands and the extension system.
func GenerateManifest(container Container) ContainerInfo {
	info := container.Info()

	var publicSlots []SlotInfo
	seen := make(map[SlotType]bool)
	var slotTypes []SlotTypeContract

	for _, s := range info.Slots {
		if s.Tags[TagSlotInternal] == "true" {
			continue
		}

		// Auto-generate slot_types from typed public slots. First slot of a
		// given type wins — since all slots of the same type must share the
		// same method contract, any one of them has the full definition.
		if s.SlotType != "" && !seen[s.SlotType] {
			seen[s.SlotType] = true
			slotTypes = append(slotTypes, SlotTypeContract{
				Type:    s.SlotType,
				Version: "1.0",
				Methods: s.Methods,
			})
		}

		// Typed slots drop their method list on the wire — it lives on the
		// slot_type. Untyped slots keep theirs as a legacy fallback.
		slotEntry := s
		if s.SlotType != "" {
			slotEntry.Methods = nil
		}
		publicSlots = append(publicSlots, slotEntry)
	}

	return ContainerInfo{
		Name:            info.Name,
		Description:     info.Description,
		Version:         info.Version,
		ProtocolVersion: KTSPProtocolVersion, // stamp the wire-protocol major
		SlotTypes:       slotTypes,
		Slots:           publicSlots,
	}
}

// GenerateManifestJSON produces the JSON representation of the manifest.
func GenerateManifestJSON(container Container) (json.RawMessage, error) {
	manifest := GenerateManifest(container)
	return json.MarshalIndent(manifest, "", "  ")
}
