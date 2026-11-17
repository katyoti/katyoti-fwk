// SPDX-License-Identifier: Apache-2.0

package contribapi

import (
	"strings"
	"testing"
)

func TestJudgeFormat(t *testing.T) {
	for _, row := range []struct {
		host, member string
		want         FormatVerdict
	}{
		{"1.2", "1.2", FormatStrict},
		{"1.2", "1.1", FormatStrict},
		{"1.2", "0.9", FormatStrict},
		{"1.2", "1.3", FormatNewerMinor},
		{"1.2", "1.3.7", FormatNewerMinor},
		{"1.2", "2.0", FormatNewerMajor},
		{"1.2", "", FormatStrict},
		{"", "9.9", FormatStrict},
		{"1.2", "dev", FormatStrict},
	} {
		if got := JudgeFormat(row.host, row.member); got != row.want {
			t.Errorf("JudgeFormat(%q, %q) = %v, want %v", row.host, row.member, got, row.want)
		}
	}
}

// The formats a binary writes are those of the kinds its own entries are of,
// read from each kind's declaration and data-type — never from the system
// entries themselves; and they travel in its sidecar-info.
func TestDeriveFormatsIntoSidecarInfo(t *testing.T) {
	decl := Contribution{Provider: SystemProvider, Type: ResourceTypeContributionType, ID: "hook"}
	decl.SetData([]byte(`{"name":"hook","schema":"system:/data-type/hook","layers":{"data":{"mediaType":"application/json","required":true,"default":true}},"meta":{"budget":100}}`))
	dt := Contribution{Provider: SystemProvider, Type: ResourceTypeDataType, ID: "hook"}
	dt.SetSource([]byte("name: hook\nversion: 1.4.2\ntypes: {}\n"), "application/yaml")
	info := Contribution{Provider: "ktw-x", Type: SidecarInfoKind, ID: "default"}
	info.SetData([]byte(`{"name":"ktw-x","version":"1"}`))
	hook := Contribution{Provider: "ktw-x", Type: "hook", ID: "h"}
	hook.SetData([]byte(`{"name":"h"}`))

	formats := DeriveFormats([]Contribution{decl, dt, info, hook})
	if len(formats) != 1 || formats["hook"] != "1.4" {
		t.Fatalf("DeriveFormats = %v, want {hook: 1.4} (the system entries are not this binary's own)", formats)
	}
	if err := SetFormats(&info, formats); err != nil {
		t.Fatal(err)
	}
	if got := MemberFormats([]Contribution{hook, info}); got["hook"] != "1.4" {
		t.Errorf("MemberFormats = %v", got)
	}
	if !strings.Contains(string(info.Data()), `"name":"ktw-x"`) {
		t.Errorf("SetFormats lost the identity: %s", info.Data())
	}
	if MemberFormats([]Contribution{hook}) != nil {
		t.Errorf("a member with no sidecar-info states no format")
	}
}

func TestFormatFindings(t *testing.T) {
	refused, tolerated := FormatFindings("ktw-x", map[string]string{"hook": "1.0", "skill": "3.0"},
		map[string]string{"hook": "2.0", "skill": "3.1", "recipe": "7.0"})
	if len(refused) != 1 || !strings.Contains(refused[0], "update the hub") {
		t.Errorf("refused = %v", refused)
	}
	if len(tolerated) != 1 || !strings.Contains(tolerated[0], "skill") {
		t.Errorf("tolerated = %v", tolerated)
	}
}
