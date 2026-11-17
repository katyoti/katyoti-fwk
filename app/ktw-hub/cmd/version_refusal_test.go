// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	manifestlocal "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/local"
)

// : the hub refuses a member whose binary is not the version its
// universe pins, naming both; a member at its pin, a dev build and a member
// whose version is not known yet run as before.
func TestAMemberAtAnotherVersionIsRefused(t *testing.T) {
	dir := t.TempDir()
	cache := manifestlocal.NewCache(filepath.Join(dir, "cache"))
	member := func(version, reported string) appcore.SidecarSpec {
		exe := filepath.Join(dir, "store", "ktw-x", version, "ktw-x.exe")
		if err := os.MkdirAll(filepath.Dir(exe), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(exe, []byte("binary "+version), 0o755); err != nil {
			t.Fatal(err)
		}
		if reported != "" {
			if err := cache.Save("ktw-x", &mbapi.Manifest{Provider: "ktw-x", Version: reported, Hash: "h-" + version}, exe, manifestlocal.SourceDigests{Binary: exe}); err != nil {
				t.Fatal(err)
			}
		}
		return appcore.SidecarSpec{Name: "ktw-x", Transport: "nats", Binary: exe}
	}

	skewed := member("v1.0.0", "v1.1.0")
	why := versionRefusal(skewed, cache)
	if !strings.Contains(why, "reports version v1.1.0") || !strings.Contains(why, "pins v1.0.0") {
		t.Fatalf("refusal = %q; want both versions named", why)
	}
	if spec := withVersionCheck(spawnSpecOf(skewed), skewed, cache); spec.Refused != why {
		t.Fatalf("the spawn spec does not carry the refusal: %+v", spec)
	}

	for name, s := range map[string]appcore.SidecarSpec{
		"at its pin":         member("v2.0.0", "v2.0.0"),
		"at its pin, no v":   member("v3.0.0", "3.0.0"),
		"a dev build":        member("v4.0.0", "dev"),
		"version not known":  member("v5.0.0", ""),
		"not from the store": {Name: "ktw-x", Transport: "nats", Binary: filepath.Join(dir, "bin", "ktw-x.exe")},
	} {
		if why := versionRefusal(s, cache); why != "" {
			t.Errorf("%s: refused %q", name, why)
		}
	}
}
