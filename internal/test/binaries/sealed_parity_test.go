// SPDX-License-Identifier: Apache-2.0

package binaries

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/appcore/frozenhost"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/test/hubharness"
)

// TestSealedPackPublishesTheServedFolderManifest is goal G25's probe: the same
// folder served live by the generic host and sealed into an executable publish
// byte-identical manifests, except the mutable flag — the ONE thing sealing is
// allowed to change. Both manifests come out of a real process (`sc resources
// manifest`), so the comparison covers everything a consumer can see: URIs,
// layers and their hashes, provider identity, the reported version.
func TestSealedPackPublishesTheServedFolderManifest(t *testing.T) {
	bins := builtBinaries(t)
	host := bins["ktw-host"]
	if host == "" {
		hubharness.MissingBinary(t, "sealed parity: bin/ktw-host not built")
		return
	}
	folder := parityPack(t)

	served := hostManifestOf(t, "served", host, "--folder", folder)

	hostBytes, err := os.ReadFile(host)
	if err != nil {
		t.Fatal(err)
	}
	frozen, err := frozenhost.Pack(hostBytes, folder)
	if err != nil {
		t.Fatalf("seal the folder: %v", err)
	}
	exe := filepath.Join(t.TempDir(), "parity-pack"+filepath.Ext(host))
	if err := os.WriteFile(exe, frozen, 0o755); err != nil {
		t.Fatal(err)
	}
	sealed := hostManifestOf(t, "sealed", exe)

	if served.Provider != "parity-pack" || sealed.Provider != "parity-pack" {
		t.Fatalf("both hosts must adopt the pack's identity: served=%q sealed=%q", served.Provider, sealed.Provider)
	}
	if served.Version != sealed.Version {
		t.Errorf("version: served %q, sealed %q — sealing must not change what the content says it is", served.Version, sealed.Version)
	}
	if a, b := shapeOf(served), shapeOf(sealed); a != b {
		t.Errorf("the sealed pack publishes a different index than the served folder:\n--- served ---\n%s\n--- sealed ---\n%s", a, b)
	}
	if served.Hash == "" || served.Hash != sealed.Hash {
		t.Errorf("manifest hash: served %q, sealed %q — the content address must be the same content", served.Hash, sealed.Hash)
	}
	// Mutability is a claim about the SOURCE: the folder-backed providers (the
	// pack's skill and reference) are mutable while served and immutable once
	// sealed; the computed and system providers are immutable either way.
	var servedMutable []string
	for _, p := range served.Providers {
		if p.Mutable {
			servedMutable = append(servedMutable, p.Type)
		}
	}
	sort.Strings(servedMutable)
	if got, want := strings.Join(servedMutable, ","), "reference,skill"; got != want {
		t.Errorf("served folder: mutable providers = %q, want %q — the folder-backed kinds and only those", got, want)
	}
	for _, p := range sealed.Providers {
		if p.Mutable {
			t.Errorf("sealed pack: provider %s/%s reads mutable; content inside an executable cannot change", p.Name, p.Type)
		}
	}
	t.Logf("sealed parity: %d contributions, %d providers, hash %s identical; only the mutable flag differs",
		len(served.Contributions), len(served.Providers), served.Hash)
}

// shapeOf renders everything a consumer can see of a manifest, minus the one
// field sealing is allowed to change (Providers[].Mutable).
func shapeOf(m *mbapi.Manifest) string {
	var lines []string
	for _, c := range m.Contributions {
		var layers []string
		for name, l := range c.Layers {
			layers = append(layers, fmt.Sprintf("%s=%s/%d/%s", name, l.Hash, l.Size, l.MediaType))
		}
		sort.Strings(layers)
		lines = append(lines, c.ResolvedURI()+"\t"+strings.Join(layers, ","))
	}
	for _, p := range m.Providers {
		// The row, not its identity: identity (mutable, digest) is the one
		// thing sealing changes (a served folder is mutable with no digest, a
		// sealed one immutable with its enumeration epoch), checked on its
		// own below.
		lines = append(lines, fmt.Sprintf("provider:%s/%s", p.Name, p.Type))
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

// parityPack writes a small files-only pack: an identity, one skill, one
// reference — enough for two providers and two kinds to have to agree.
func parityPack(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("ktw-host.yaml", "name: parity-pack\nversion: 1.0.0\ndescription: the same content, served or sealed\n")
	write("skills/parity-skill/SKILL.md", "---\nname: parity-skill\ndescription: a skill that must read the same either way\n---\n\nRead me either way.\n")
	write("references/parity-guide.md", "# Parity guide\n\nOne file, two hosts, one hash.\n")
	return dir
}

// hostManifestOf runs a host binary with the given leading args and reads the
// manifest it publishes. The sealed host takes no args: it finds its content
// inside itself.
func hostManifestOf(t *testing.T, label, binary string, args ...string) *mbapi.Manifest {
	t.Helper()
	home := tempWorkspace(t, "parity-"+label)
	full := append(append([]string{}, args...), "sc", "resources", "manifest", "--out", "json")
	cmd := exec.Command(binary, full...)
	cmd.Env = append(os.Environ(), "KTW_WORKSPACE="+home, "KTW_DATA="+home)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	done := make(chan struct{})
	var out []byte
	var runErr error
	go func() {
		out, runErr = cmd.Output()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(manifestTimeout):
		_ = cmd.Process.Kill()
		t.Fatalf("%s host: `sc resources manifest` did not answer within %s\n--- stderr ---\n%s", label, manifestTimeout, stderr.String())
	}
	if runErr != nil {
		t.Fatalf("%s host: `sc resources manifest` failed: %v\n--- stderr ---\n%s", label, runErr, stderr.String())
	}
	var m mbapi.Manifest
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("%s host: manifest is not JSON: %v", label, err)
	}
	return &m
}
