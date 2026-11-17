// SPDX-License-Identifier: Apache-2.0

package core

import (
	"strings"
	"testing"
)

// TestKtspArgs pins the one meaning `sidecars[].args` has: the WHOLE argv.
//
// Every production writer treats it that way, the content-pack path included —
// `["--folder", <path>, "sc", "ktsp", "nats"]` is the host's flags AND the
// invocation in one list. A test once declared only `--folder <path>`, and the
// spawner briefly grew a sniff to accommodate it; that ran the child as
// `ktw-host --folder X` with no subcommand — help, exit, `transport: closed`.
// The test was wrong, not the contract.
func TestKtspArgs(t *testing.T) {
	cases := []struct {
		name     string
		declared []string
		want     string
	}{
		{
			name:     "nothing declared gets the plain invocation",
			declared: nil,
			want:     "sc ktsp stdio",
		},
		{
			name:     "a declared argv is used verbatim",
			declared: []string{"sc", "ktsp", "stdio"},
			want:     "sc ktsp stdio",
		},
		{
			name: "flags and invocation travel together",
			// The content-pack shape: the host needs the folder AND has to be
			// told to speak KTSP, and the writer states both.
			declared: []string{"--folder", "/packs/live", "sc", "ktsp", "stdio"},
			want:     "--folder /packs/live sc ktsp stdio",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := strings.Join(ktspArgs(tc.declared, "stdio"), " ")
			if got != tc.want {
				t.Errorf("ktspArgs(%v) = %q, want %q", tc.declared, got, tc.want)
			}
		})
	}
}

// The transport is not hardcoded — the NATS spawner asks for its own.
func TestKtspArgs_Transport(t *testing.T) {
	if got := strings.Join(ktspArgs(nil, "nats"), " "); got != "sc ktsp nats" {
		t.Errorf("got %q", got)
	}
}

// ktspArgs must not write through the caller's slice: the spec is reused for
// every restart.
func TestKtspArgs_DoesNotMutateTheSpec(t *testing.T) {
	declared := []string{"--folder", "/p", "sc", "ktsp", "stdio"}
	_ = ktspArgs(declared, "stdio")
	_ = ktspArgs(declared, "stdio")
	if len(declared) != 5 {
		t.Fatalf("the declared args changed to %v", declared)
	}
}
