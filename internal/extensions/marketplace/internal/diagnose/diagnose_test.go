// SPDX-License-Identifier: Apache-2.0

package diagnose

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
)

// world is a temporary installation (never the person's): its registries,
// its store, its control plane and a folder standing for the temporary one.
type world struct {
	base, root, store, cp, tmp string
}

func newWorld(t *testing.T) world {
	t.Helper()
	base := t.TempDir()
	w := world{base: base, root: filepath.Join(base, "root"), store: filepath.Join(base, "root", "bin", "store"),
		cp: filepath.Join(base, "root", "control-plane"), tmp: filepath.Join(base, "tmp")}
	for _, d := range []string{w.root, w.store, w.cp, w.tmp} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return w
}

func (w world) files() Files {
	return Files{
		Instances:    filepath.Join(w.base, "katyoti-instances.yaml"),
		Universes:    filepath.Join(w.root, "universes.yaml"),
		Sidecars:     filepath.Join(w.root, "sidecars.yaml"),
		Marketplaces: filepath.Join(w.root, "marketplaces.yaml"),
		ControlPlane: w.cp,
		TempDirs:     []string{w.tmp},
	}
}

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func sum(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// q quotes a path for YAML (Windows backslashes stay literal).
func q(p string) string { return "'" + filepath.ToSlash(p) + "'" }

func exe(name string) string { return name + exeSuffix() }

// A sound installation has no problem: programs match their checksums and
// every row names a path that is there.
func TestDiagnose_SoundInstallationHasNoProblem(t *testing.T) {
	w := newWorld(t)
	f := w.files()
	good := filepath.Join(w.store, "ktw-a", "1.0", exe("ktw-a"))
	write(t, good, "a-bytes")
	cpStore := filepath.Join(w.store, "katyoti", "2.0", exe("katyoti"))
	write(t, cpStore, "katyoti-bytes")
	write(t, filepath.Join(w.cp, exe("katyoti")), "katyoti-bytes")
	write(t, f.Sidecars, "sidecars:\n"+
		"  - {name: ktw-a, version: '1.0', binary_path: "+q(good)+", sha256: "+sum("a-bytes")+"}\n"+
		"  - {name: katyoti, version: '2.0', source: infra-channel, binary_path: "+q(cpStore)+", sha256: "+sum("katyoti-bytes")+"}\n"+
		"  - {name: ktw-on-path, binary_path: ktw-on-path}\n")
	write(t, f.Universes, "universes:\n  - {name: dev, home_path: "+q(w.root)+"}\n")
	write(t, f.Instances, "instances:\n  - {name: dev, folder: "+q(w.base)+"}\n")
	write(t, f.Marketplaces, "marketplaces:\n  - {name: acme, url: 'https://example.com/m'}\n  - {name: seeded, enabled: false}\n")

	d := Diagnose(f)
	if len(d.Problems) != 0 {
		t.Fatalf("a sound installation is diagnosed with problems: %+v", d.Problems)
	}
	if len(d.Checked) != 4 {
		t.Fatalf("checked %v, want the four registries", d.Checked)
	}
}

// Each broken thing is named, with its kind, file and row: a program
// replaced or gone against its checksum (in the store or the control plane),
// a registry row to a vanished path in each registry, a row a test left.
func TestDiagnose_NamesEachBrokenThing(t *testing.T) {
	w := newWorld(t)
	f := w.files()
	gone := filepath.Join(w.base, "gone")

	replaced := filepath.Join(w.store, "ktw-b", "1.0", exe("ktw-b"))
	write(t, replaced, "other-bytes")
	cpStore := filepath.Join(w.store, "ktw-hub", "2.0", exe("ktw-hub"))
	write(t, cpStore, "hub-bytes")
	write(t, filepath.Join(w.cp, exe("ktw-hub")), "a-dev-build")
	write(t, f.Sidecars, "sidecars:\n"+
		"  - {name: ktw-b, version: '1.0', binary_path: "+q(replaced)+", sha256: "+sum("b-bytes")+"}\n"+
		"  - {name: ktw-c, version: '1.0', binary_path: "+q(filepath.Join(gone, exe("ktw-c")))+", sha256: "+sum("c")+"}\n"+
		"  - {name: ktw-d, binary_path: "+q(filepath.Join(gone, exe("ktw-d")))+"}\n"+
		"  - {name: ktw-hub, version: '2.0', source: infra-channel, binary_path: "+q(cpStore)+", sha256: "+sum("hub-bytes")+"}\n")

	testUniverse := filepath.Join(w.tmp, "TestX123", "001", "u")
	if err := os.MkdirAll(testUniverse, 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, f.Universes, "universes:\n"+
		"  - {name: dev, home_path: "+q(w.root)+"}\n"+
		"  - {name: old, home_path: "+q(filepath.Join(gone, "old"))+"}\n"+
		"  - {name: scn-test, home_path: "+q(testUniverse)+"}\n")
	write(t, f.Instances, "instances:\n"+
		"  - {name: dev, folder: "+q(w.base)+"}\n"+
		"  - {name: removed, folder: "+q(filepath.Join(gone, "Katyoti"))+"}\n")
	write(t, f.Marketplaces, "marketplaces:\n"+
		"  - {name: acme, url: 'https://example.com/m'}\n"+
		"  - {name: httptest, url: 'http://127.0.0.1:53111/katyoti-marketplace.yaml'}\n"+
		"  - {name: tmp-folder, url: "+q(filepath.Join(w.tmp, "mkt"))+"}\n"+
		"  - {name: moved, url: 'file://"+filepath.ToSlash(filepath.Join(gone, "mkt"))+"'}\n")

	want := map[string]string{ // entry -> kind
		"ktw-b":      mktapi.ProblemProgramReplaced,
		"ktw-c":      mktapi.ProblemProgramMissing,
		"ktw-d":      mktapi.ProblemVanishedPath,
		"ktw-hub":    mktapi.ProblemProgramReplaced,
		"old":        mktapi.ProblemVanishedPath,
		"scn-test":   mktapi.ProblemTestEntry,
		"removed":    mktapi.ProblemVanishedPath,
		"httptest":   mktapi.ProblemTestEntry,
		"tmp-folder": mktapi.ProblemTestEntry,
		"moved":      mktapi.ProblemVanishedPath,
	}
	d := Diagnose(f)
	got := map[string]mktapi.InstallationProblem{}
	for _, p := range d.Problems {
		if _, dup := got[p.Entry]; dup {
			t.Errorf("%s named twice", p.Entry)
		}
		got[p.Entry] = p
	}
	for entry, kind := range want {
		p, ok := got[entry]
		if !ok {
			t.Errorf("%s is not named (problems: %+v)", entry, d.Problems)
			continue
		}
		if p.Kind != kind {
			t.Errorf("%s: kind %s, want %s (%s)", entry, p.Kind, kind, p.Detail)
		}
		if p.File == "" || p.Detail == "" || p.Repair == "" {
			t.Errorf("%s: a problem says its file, what is wrong and the repair: %+v", entry, p)
		}
	}
	if len(got) != len(want) {
		t.Errorf("named %d problems, want %d: %+v", len(got), len(want), d.Problems)
	}
	if p := got["ktw-hub"]; !strings.EqualFold(p.File, filepath.Join(w.cp, exe("ktw-hub"))) {
		t.Errorf("the replaced control-plane program names its own path, got %s", p.File)
	}
	if p := got["removed"]; !strings.EqualFold(p.File, f.Instances) {
		t.Errorf("a vanished installation names katyoti-instances.yaml, got %s", p.File)
	}
}

// Without temporary folders (an installation that itself lives in one), a
// row there is no test leftover.
func TestDiagnose_NoTempDirsNoTestEntry(t *testing.T) {
	w := newWorld(t)
	f := w.files()
	f.TempDirs = nil
	write(t, f.Universes, "universes:\n  - {name: u, home_path: "+q(w.tmp)+"}\n")
	if d := Diagnose(f); len(d.Problems) != 0 {
		t.Fatalf("problems %+v, want none", d.Problems)
	}
}

// A registry that cannot be read is named, and the others are still read.
func TestDiagnose_UnreadableRegistryIsNamed(t *testing.T) {
	w := newWorld(t)
	f := w.files()
	write(t, f.Universes, "universes: [ {name: : }\n")
	write(t, f.Instances, "instances:\n  - {name: removed, folder: "+q(filepath.Join(w.base, "gone"))+"}\n")
	d := Diagnose(f)
	if len(d.Problems) != 2 {
		t.Fatalf("problems %+v, want the unreadable universes.yaml and the vanished installation", d.Problems)
	}
}
