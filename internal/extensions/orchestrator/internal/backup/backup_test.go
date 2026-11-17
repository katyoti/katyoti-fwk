// SPDX-License-Identifier: Apache-2.0

package backup

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func tarNames(t *testing.T, path string) []string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(zr)
	var names []string
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, h.Name)
	}
	sort.Strings(names)
	return names
}

// universe lays out a made-up universe folder: two configuration files, a
// credentials file the allow-list would match, a subfolder never copied, and a
// scheduler snapshot.
func universe(t *testing.T) (cfg, snap string) {
	root := t.TempDir()
	cfg = filepath.Join(root, "home")
	write(t, filepath.Join(cfg, "Univers.yaml"), "name: example\n")
	write(t, filepath.Join(cfg, "LLMProfiles.yaml"), "profiles: []\n")
	write(t, filepath.Join(cfg, ".env"), "FAKE=1\n")
	write(t, filepath.Join(cfg, ".env.yaml"), "FAKE=1\n")
	write(t, filepath.Join(cfg, "sessions", "s.yaml"), "x\n")
	snap = filepath.Join(root, "scheduler-snap.db")
	write(t, snap, "SQLite format 3\x00 fake")
	return cfg, snap
}

func TestRunWritesSealedCopiesOfTheAllowListAndTheSnapshot(t *testing.T) {
	cfg, snap := universe(t)
	local, off := filepath.Join(t.TempDir(), "local"), filepath.Join(t.TempDir(), "off")
	now := time.Date(2026, 11, 17, 8, 0, 0, 0, time.UTC)
	res, err := Run(Job{Universe: "ex", ConfigDir: cfg, Allow: []string{"*.yaml", ".env*"}, Snapshot: snap, Local: local, OffPC: off, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if !res.OffPC || len(res.Written) != 4 {
		t.Fatalf("first run of the day: want 4 archives and the off-PC copy, got %+v", res)
	}
	for _, p := range res.Written {
		if err := Verify(p); err != nil {
			t.Fatalf("%s: %v", p, err)
		}
	}
	got := strings.Join(tarNames(t, filepath.Join(local, "ex-20261117T080000Z-config.tar.gz")), ",")
	if got != "LLMProfiles.yaml,Univers.yaml" {
		t.Fatalf("the config archive holds exactly the allow-list without credentials, got %s", got)
	}
	// A second run the same day writes locally only.
	res, err = Run(Job{Universe: "ex", ConfigDir: cfg, Snapshot: snap, Local: local, OffPC: off, Now: now.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if res.OffPC || len(res.Written) != 2 {
		t.Fatalf("second run of the day: want local only, got %+v", res)
	}
}

func TestRunNeverSendsALocalOnlyUniverseOffThePC(t *testing.T) {
	cfg, snap := universe(t)
	local, off := t.TempDir(), filepath.Join(t.TempDir(), "off")
	if _, err := Run(Job{Universe: "ex", ConfigDir: cfg, Snapshot: snap, Local: local, OffPC: off, LocalOnly: true, Now: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(off); !os.IsNotExist(err) {
		t.Fatalf("a local-only universe wrote to the off-PC target")
	}
}

func TestVerifyRefusesADamagedArchive(t *testing.T) {
	cfg, snap := universe(t)
	res, err := Run(Job{Universe: "ex", ConfigDir: cfg, Snapshot: snap, Local: t.TempDir(), Now: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(res.Written[0], []byte("damaged"), 0o644); err != nil {
		t.Fatal(err)
	}
	if Verify(res.Written[0]) == nil {
		t.Fatal("a damaged archive passed its sha256")
	}
}

func TestPruneKeeps24HourlyAnd14Daily(t *testing.T) {
	cfg, snap := universe(t)
	local := t.TempDir()
	start := time.Date(2026, 11, 1, 0, 30, 0, 0, time.UTC)
	var now time.Time
	for h := 0; h < 20*24; h++ { // 20 days, hourly
		now = start.Add(time.Duration(h) * time.Hour)
		if _, err := Run(Job{Universe: "ex", ConfigDir: cfg, Snapshot: snap, Local: local, Now: now}); err != nil {
			t.Fatal(err)
		}
	}
	gens, err := generations(local, "ex")
	if err != nil {
		t.Fatal(err)
	}
	// 24 hourly, all on the last day, which is also one of the 14 daily
	// slots: the 13 other days keep one generation each.
	if len(gens) != KeepHourly+KeepDaily-1 {
		t.Fatalf("want %d generations kept, got %d", KeepHourly+KeepDaily-1, len(gens))
	}
	if oldest := gens[len(gens)-1].at; now.Sub(oldest) >= KeepDaily*24*time.Hour {
		t.Fatalf("a generation older than %d days is kept: %s", KeepDaily, oldest)
	}
}

func TestIsSecretFile(t *testing.T) {
	for name, want := range map[string]bool{".env": true, ".env.build": true, ".env.build.example": false, "env.yaml": false, "Univers.yaml": false} {
		if IsSecretFile(name) != want {
			t.Errorf("IsSecretFile(%q) = %v", name, !want)
		}
	}
}

// A rename that fails leaves neither the archive's part nor a sum without its
// archive: a directory standing at the archive's name (or at its sum's)
// makes the rename fail.
func TestSealLeavesNoStraySumWhenARenameFails(t *testing.T) {
	for _, blocked := range []string{"", ".sha256"} {
		dir := t.TempDir()
		path := filepath.Join(dir, "u-20261117T060000Z.db.gz")
		if err := os.MkdirAll(filepath.Join(path+blocked, "x"), 0o755); err != nil {
			t.Fatal(err)
		}
		err := seal(path, func(w io.Writer) error { _, err := w.Write([]byte("data")); return err })
		if err == nil {
			t.Fatalf("blocked %q: seal succeeded over a directory", blocked)
		}
		for _, stray := range []string{path + ".part", path + ".sha256.part"} {
			if _, err := os.Stat(stray); err == nil {
				t.Errorf("blocked %q: %s left behind", blocked, filepath.Base(stray))
			}
		}
		if blocked == "" {
			if _, err := os.Stat(path + ".sha256"); err == nil {
				t.Errorf("a .sha256 stands without its archive")
			}
		} else if st, err := os.Stat(path); err == nil && st.Mode().IsRegular() {
			t.Errorf("an archive stands without its .sha256")
		}
	}
}

// A credential shape in an allow-listed file refuses the whole run before any
// target is written; the same folder without it seals. The
// made-up token is built at run time so no shape sits in this source.
func TestRunRefusesAConfigFileCarryingASecret(t *testing.T) {
	cfg, snap := universe(t)
	token := "gh" + "p_" + strings.Repeat("a1", 18)
	write(t, filepath.Join(cfg, "Remotes.yaml"), "url: https://example.org/repo\ntoken: "+token+"\n")
	local, off := filepath.Join(t.TempDir(), "local"), filepath.Join(t.TempDir(), "off")
	job := Job{Universe: "ex", ConfigDir: cfg, Snapshot: snap, Local: local, OffPC: off, Now: time.Date(2026, 11, 17, 8, 0, 0, 0, time.UTC)}
	res, err := Run(job)
	if err == nil {
		t.Fatalf("a configuration file carrying a secret was copied: %+v", res)
	}
	if !strings.Contains(err.Error(), "Remotes.yaml:2") || strings.Contains(err.Error(), token) {
		t.Fatalf("the refusal names the file and line, never the value: %v", err)
	}
	for _, dir := range []string{local, off} {
		if entries, _ := os.ReadDir(dir); len(entries) > 0 {
			t.Fatalf("a refused run left %d files in %s", len(entries), dir)
		}
	}
	write(t, filepath.Join(cfg, "Remotes.yaml"), "url: https://example.org/repo\n")
	res, err = Run(job)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Written) != 4 {
		t.Fatalf("a clean folder seals the snapshot and the configuration in both targets, got %+v", res)
	}
}
