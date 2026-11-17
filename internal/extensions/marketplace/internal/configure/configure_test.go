// SPDX-License-Identifier: Apache-2.0

package configure

import "testing"

// A refused write comes back as the sidecar's own message, not the Cobra
// usage block that follows it on stderr.
func TestFirstErrorLine_StripsCobraUsage(t *testing.T) {
	stderr := "Error: config \"X.yaml\" rejected: invalid YAML: yaml: line 1: did not find expected ',' or ']'\nUsage:\n  ktw-x sidecar config set [flags]\n\nFlags:\n      --file string\n"
	got := firstErrorLine(stderr)
	want := "config \"X.yaml\" rejected: invalid YAML: yaml: line 1: did not find expected ',' or ']'"
	if got != want {
		t.Errorf("got %q\nwant %q", got, want)
	}
	if firstErrorLine("\n   \n") != "" {
		t.Error("blank stderr should yield an empty message (caller falls back to the exec error)")
	}
	if firstErrorLine("plain failure\nmore") != "plain failure" {
		t.Error("a non-Cobra first line is returned as-is")
	}
}
