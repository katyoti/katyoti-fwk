// SPDX-License-Identifier: Apache-2.0

package appcore

import (
	"strings"
	"testing"
)

// TestMemberManifestArgs_NeverAskForLocalEntries — blind audit M3: the
// aggregator's probe of a member is the process boundary a `local:` entry
// never crosses, so the invocation it renders always says so. The CLI
// defaults both toggles to true (a binary's SELF view is complete), which is
// exactly why the aggregator must pass them and never inherit them.
func TestMemberManifestArgs_NeverAskForLocalEntries(t *testing.T) {
	for _, includeSystem := range []bool{false, true} {
		got := strings.Join(memberManifestArgs(MemberManifestRequest(includeSystem)), " ")
		if !strings.HasPrefix(got, "sc resources manifest ") {
			t.Fatalf("includeSystem=%t: probe %q is not `sc resources manifest`", includeSystem, got)
		}
		if !strings.Contains(got, "--include-local=false") {
			t.Errorf("includeSystem=%t: probe %q leaves --include-local at the CLI default (true): local entries would cross the boundary", includeSystem, got)
		}
		if want := "--include-system=" + map[bool]string{false: "false", true: "true"}[includeSystem]; !strings.Contains(got, want) {
			t.Errorf("includeSystem=%t: probe %q does not pass %s", includeSystem, got, want)
		}
	}
	// The request itself, as the census asks it in process: local never.
	in := MemberManifestRequest(false)
	if in.IncludeLocal == nil || *in.IncludeLocal {
		t.Errorf("MemberManifestRequest.IncludeLocal = %v, want an explicit false", in.IncludeLocal)
	}
}

// TestIsMemberManifestProbe: the process the probe starts recognises itself
// from the very invocation the probe renders, global flags or not — and no
// other command reads as the probe.
func TestIsMemberManifestProbe(t *testing.T) {
	probe := memberManifestArgs(MemberManifestRequest(false))
	if !IsMemberManifestProbe(probe) {
		t.Fatalf("%q is not recognised as the probe", probe)
	}
	if !IsMemberManifestProbe(append([]string{"--universe", "u"}, probe...)) {
		t.Error("the probe behind a global flag is not recognised")
	}
	for _, other := range [][]string{
		{"sc", "resources", "list"},
		{"sc", "ktsp", "nats"},
		{"manifest"},
		nil,
	} {
		if IsMemberManifestProbe(other) {
			t.Errorf("%q reads as the probe", other)
		}
	}
}
