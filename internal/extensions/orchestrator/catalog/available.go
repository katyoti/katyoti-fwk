// SPDX-License-Identifier: Apache-2.0

package catalog

// AvailableSidecars is the list a universe can be composed from: what is
// INSTALLED on this machine, then what a live scan discovered, minus the
// control plane.
//
// The universe editor used to show the raw scan alone. On a developer box that
// is everything (bin/ is on PATH); on a machine built from the channel it is
// exactly the control-plane folder — so the editor offered ktw-hub and
// ktw-univers, which are not sidecars, and none of the items the user had just
// installed from the marketplace, which live in the versioned store and sit on
// no PATH at all.
//
// Rules, in order:
//   - a recorded row with a managed source (marketplace, infra-channel, …) and
//     a binary path is a real install and comes FIRST, with its version and
//     provenance intact. Rows without a binary (a `content` item runs on
//     ktw-host with a folder) cannot be expressed as a bare binary selection and
//     are left to the marketplace surface, which knows how to wire them;
//   - a scanned row fills in a name the record does not have (a dev binary);
//   - excluded names (the control plane) never appear, from either side.
func AvailableSidecars(recorded, scanned []SidecarsYamlEntry, exclude func(name string) bool) []SidecarsYamlEntry {
	if exclude == nil {
		exclude = func(string) bool { return false }
	}
	seen := map[string]bool{}
	out := make([]SidecarsYamlEntry, 0, len(recorded)+len(scanned))
	for _, r := range recorded {
		if exclude(r.Name) || seen[r.Name] {
			continue
		}
		if !IsManagedSource(r.Source) || r.BinaryPath == "" {
			continue
		}
		if r.Kind != "" && r.Kind != "sidecar" {
			continue
		}
		seen[r.Name] = true
		out = append(out, r)
	}
	for _, s := range scanned {
		if exclude(s.Name) || seen[s.Name] {
			continue
		}
		seen[s.Name] = true
		out = append(out, s)
	}
	return out
}
