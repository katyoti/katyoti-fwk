// SPDX-License-Identifier: Apache-2.0

package catalog

import "testing"

// The situation this exists for: a machine built from the channel. The record
// holds the four control-plane rows + two marketplace installs; the scan sees
// only the control-plane folder. The editor must offer the two installs and
// nothing from the control plane.
func TestAvailableSidecars_FreshMachine(t *testing.T) {
	recorded := []SidecarsYamlEntry{
		{Name: "katyoti", BinaryPath: `D:\root\bin\store\katyoti\v1\katyoti.exe`, Version: "v1", Source: "infra-channel", Kind: "sidecar"},
		{Name: "ktw-hub", BinaryPath: `D:\root\bin\store\ktw-hub\v1\ktw-hub.exe`, Version: "v1", Source: "infra-channel", Kind: "sidecar"},
		{Name: "ktw-example", BinaryPath: `D:\root\bin\store\ktw-example\v1\ktw-example.exe`, Version: "v1", Source: "marketplace", Kind: "sidecar"},
		{Name: "acme-playbooks", FolderPath: `D:\root\bin\store-content\acme-playbooks\v1`, Version: "v1", Source: "marketplace", Kind: "content"},
	}
	scanned := []SidecarsYamlEntry{
		{Name: "ktw-hub", BinaryPath: `D:\root\control-plane\ktw-hub.exe`, Source: SourceExecutableDir},
		{Name: "ktw-univers", BinaryPath: `D:\root\control-plane\ktw-univers.exe`, Source: SourceExecutableDir},
	}
	infra := map[string]bool{"katyoti": true, "ktw-hub": true, "ktw-gui": true, "ktw-univers": true}

	got := AvailableSidecars(recorded, scanned, func(n string) bool { return infra[n] })
	if len(got) != 1 || got[0].Name != "ktw-example" {
		t.Fatalf("want exactly the marketplace install, got %+v", got)
	}
	if got[0].Version != "v1" || got[0].Source != "marketplace" || got[0].BinaryPath == "" {
		t.Fatalf("the recorded row must keep its provenance: %+v", got[0])
	}
}

// A developer box: the record is empty, the scan finds dev binaries; they are
// offered as before. A recorded install of the same name wins over the scan.
func TestAvailableSidecars_DevBoxAndPrecedence(t *testing.T) {
	recorded := []SidecarsYamlEntry{
		{Name: "ktw-docs", BinaryPath: `/root/bin/store/ktw-docs/v2/ktw-docs`, Version: "v2", Source: "marketplace"},
		{Name: "ktw-old", BinaryPath: `/old/ktw-old`, Source: SourcePATH}, // a stale scan row, not managed
	}
	scanned := []SidecarsYamlEntry{
		{Name: "ktw-docs", BinaryPath: `/repo/bin/ktw-docs`, Source: SourcePATH},
		{Name: "ktw-notify", BinaryPath: `/repo/bin/ktw-notify`, Source: SourcePATH},
	}
	got := AvailableSidecars(recorded, scanned, nil)
	if len(got) != 2 {
		t.Fatalf("want 2 rows, got %+v", got)
	}
	if got[0].Name != "ktw-docs" || got[0].BinaryPath != `/root/bin/store/ktw-docs/v2/ktw-docs` {
		t.Fatalf("the install must win over the dev copy: %+v", got[0])
	}
	if got[1].Name != "ktw-notify" || got[1].Source != SourcePATH {
		t.Fatalf("the dev binary must still be offered: %+v", got[1])
	}
}
