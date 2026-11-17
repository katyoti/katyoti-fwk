// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/contribution/builtin"
	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
)

func TestPickVersion(t *testing.T) {
	it := mktapi.Item{Name: "x", Versions: []mktapi.ItemVersion{{Version: "2.0"}, {Version: "1.0"}}}

	if v, _, err := pickVersion(it, ""); err != nil || v != "2.0" {
		t.Fatalf("latest: want 2.0, got %q (err %v)", v, err)
	}
	if v, _, err := pickVersion(it, "1.0"); err != nil || v != "1.0" {
		t.Fatalf("explicit: want 1.0, got %q (err %v)", v, err)
	}
	if _, _, err := pickVersion(it, "9.9"); err == nil {
		t.Fatal("missing version should error")
	}
	if _, _, err := pickVersion(mktapi.Item{Name: "y"}, ""); err == nil {
		t.Fatal("no versions should error")
	}
}

func TestPickAsset(t *testing.T) {
	iv := mktapi.ItemVersion{Assets: []mktapi.Asset{
		{OS: "plan9", Arch: "amd64", URL: "other"},
		{OS: runtime.GOOS, Arch: runtime.GOARCH, URL: "me"},
	}}
	if a, err := pickAsset(iv); err != nil || a.URL != "me" {
		t.Fatalf("want current-platform asset, got %v (err %v)", a, err)
	}
	// platform-agnostic asset (no OS/arch) matches — content bundles.
	if a, err := pickAsset(mktapi.ItemVersion{Assets: []mktapi.Asset{{URL: "any"}}}); err != nil || a.URL != "any" {
		t.Fatalf("agnostic asset should match, got %v (err %v)", a, err)
	}
	if _, err := pickAsset(mktapi.ItemVersion{Assets: []mktapi.Asset{{OS: "plan9", Arch: "x"}}}); err == nil {
		t.Fatal("no matching asset should error")
	}
}

// A sidecar writing a kind in a newer MAJOR than this platform reads is
// refused BEFORE any download — the asset URL here answers nothing, so a
// refusal that downloaded first would fail on the fetch instead. A newer minor
// installs (its unknown keys are ignored at read, with a finding).
func TestInstallRefusesANewerFormatBeforeDownload(t *testing.T) {
	host := map[string]string{"hook": "1.0", "skill": "3.0"}
	if err := RefuseNewerFormats("ktw-x", "2", map[string]string{"hook": "1.4", "skill": "3.0", "recipe": "9.0"}, host); err != nil {
		t.Fatalf("a newer minor (and a kind the host does not own) must install: %v", err)
	}
	err := RefuseNewerFormats("ktw-x", "2", map[string]string{"hook": "2.0"}, host)
	if err == nil || !strings.Contains(err.Error(), "update the hub") || !strings.Contains(err.Error(), "hook") {
		t.Fatalf("a newer major: err = %v, want a refusal naming hook and saying to update the hub", err)
	}

	it := mktapi.Item{Name: "ktw-new", Kind: mktapi.KindSidecar, Versions: []mktapi.ItemVersion{{
		Version:  "9.9.9",
		Assets:   []mktapi.Asset{{OS: runtime.GOOS, Arch: runtime.GOARCH, URL: "http://127.0.0.1:1/never-fetched"}},
		Requires: mktapi.Requires{Formats: map[string]string{"hook": "99.0"}},
	}}}
	_, err = (&service{}).installSidecar(context.Background(), it, "m", "", "")
	if err == nil || !strings.Contains(err.Error(), "needs a newer hub") {
		t.Fatalf("install: err = %v, want the format refusal before any download", err)
	}
}

// The install judges a sidecar's formats against every kind this binary
// reads, a kind its pack declares (skill, task, persona) as much as a system
// one: a newer major of a pack-declared kind is refused before any download.
func TestInstallRefusesANewerMajorOfAPackDeclaredKind(t *testing.T) {
	host := builtin.Formats()
	for _, kind := range []string{"skill", "task", "persona"} {
		f, ok := host[kind]
		if !ok {
			t.Fatalf("the built-in reader does not know %q: %v", kind, host)
		}
		major, err := strconv.Atoi(strings.SplitN(f, ".", 2)[0])
		if err != nil {
			t.Fatalf("%s format %q: %v", kind, f, err)
		}
		it := mktapi.Item{Name: "ktw-new", Kind: mktapi.KindSidecar, Versions: []mktapi.ItemVersion{{
			Version:  "9.9.9",
			Assets:   []mktapi.Asset{{OS: runtime.GOOS, Arch: runtime.GOARCH, URL: "http://127.0.0.1:1/never-fetched"}},
			Requires: mktapi.Requires{Formats: map[string]string{kind: strconv.Itoa(major+1) + ".0"}},
		}}}
		_, err = (&service{}).installSidecar(context.Background(), it, "m", "", "")
		if err == nil || !strings.Contains(err.Error(), "needs a newer hub") || !strings.Contains(err.Error(), kind) {
			t.Fatalf("%s %d.0: err = %v, want the format refusal naming the kind before any download", kind, major+1, err)
		}
	}
}
