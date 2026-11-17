// SPDX-License-Identifier: Apache-2.0

package local

import (
	"context"
	"encoding/json"
	"testing"

	codeapi "github.com/katyoti/katyoti-fwk/internal/extensions/code/api"
	ktspapi "github.com/katyoti/katyoti-fwk/internal/kts/api"
)

// fakeSender records the path it was called with and returns a canned reply.
type fakeSender struct {
	gotPath  string
	gotInput any
	reply    any
}

// Call matches ktsp.Sender. The interface renamed Send -> Call and added
// variadic CallOptions on 2026-09-01; this fake was last updated 2026-06-19, so
// the package's tests had not compiled since. Restored 2026-09-17 while
// migrating code/local for p1d.
func (f *fakeSender) Call(_ context.Context, path string, params any, _ ...ktspapi.CallOption) (json.RawMessage, error) {
	f.gotPath = path
	f.gotInput = params
	return json.Marshal(f.reply)
}

func (f *fakeSender) Notify(_ context.Context, _ string, _ any, _ ...ktspapi.CallOption) error {
	return nil
}

// TestBusLanguagePack_PathsAndRoundTrip pins that each method addresses the
// EXACT `<provider>:/LanguagePack/<kebab-method>` slot and round-trips its typed
// I/O. The slot names are the shared codeapi.LPMethod* constants, so a publisher
// using the same constants matches by construction.
func TestBusLanguagePack_PathsAndRoundTrip(t *testing.T) {
	cases := []struct {
		name     string
		wantPath string
		call     func(b *BusLanguagePack, fs *fakeSender) error
	}{
		{
			name:     "Describe",
			wantPath: "ktw-gocode:/language-pack/describe",
			call: func(b *BusLanguagePack, fs *fakeSender) error {
				fs.reply = codeapi.LanguagePackInfo{Language: "go"}
				out, err := b.Describe(context.Background(), codeapi.DescribeInput{})
				if err == nil && out.Language != "go" {
					t.Errorf("Describe language = %q, want go", out.Language)
				}
				return err
			},
		},
		{
			name:     "Discover",
			wantPath: "ktw-gocode:/language-pack/discover",
			call: func(b *BusLanguagePack, fs *fakeSender) error {
				fs.reply = codeapi.DiscoverOutput{Warnings: []string{"w"}}
				_, err := b.Discover(context.Background(), codeapi.DiscoverInput{Path: "x"})
				return err
			},
		},
		{
			name:     "ProposeUCsFromTests",
			wantPath: "ktw-gocode:/language-pack/propose-u-cs-from-tests",
			call: func(b *BusLanguagePack, fs *fakeSender) error {
				fs.reply = codeapi.ProposeUCsOutput{}
				_, err := b.ProposeUCsFromTests(context.Background(), codeapi.ProposeUCsInput{})
				return err
			},
		},
		{
			name:     "ReverseEngineerFeatures",
			wantPath: "ktw-gocode:/language-pack/reverse-engineer-features",
			call: func(b *BusLanguagePack, fs *fakeSender) error {
				fs.reply = codeapi.ReverseEngineerOutput{}
				_, err := b.ReverseEngineerFeatures(context.Background(), codeapi.ReverseEngineerInput{})
				return err
			},
		},
		{
			name:     "RunUnitTest",
			wantPath: "ktw-gocode:/language-pack/run-unit-test",
			call: func(b *BusLanguagePack, fs *fakeSender) error {
				fs.reply = codeapi.RunUnitTestOutput{}
				_, err := b.RunUnitTest(context.Background(), codeapi.RunUnitTestInput{})
				return err
			},
		},
		{
			name:     "GateCheck",
			wantPath: "ktw-gocode:/language-pack/gate-check",
			call: func(b *BusLanguagePack, fs *fakeSender) error {
				fs.reply = codeapi.GateCheckOutput{}
				_, err := b.GateCheck(context.Background(), codeapi.GateCheckInput{})
				return err
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fs := &fakeSender{}
			b := NewBusLanguagePack(fs, "ktw-gocode")
			if err := tc.call(b, fs); err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			if fs.gotPath != tc.wantPath {
				t.Errorf("%s path = %q, want %q", tc.name, fs.gotPath, tc.wantPath)
			}
		})
	}
}
