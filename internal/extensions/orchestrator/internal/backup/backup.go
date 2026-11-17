// SPDX-License-Identifier: Apache-2.0

// Package backup is the universe backup job's carrier for its two file
// classes: the scheduler store's newest snapshot, copied as
// <universe>-<stamp>.db.gz, and an allow-list of the universe's configuration
// files, copied as <universe>-<stamp>-config.tar.gz - each with a .sha256 file
// read back before the copy counts. Every run writes to the local target; the
// first run of a day also writes to the off-PC target, unless the universe is
// local-only (client-named data stays on the PC). Generations are pruned to
// 24 hourly and 14 daily copies. Credentials never: a .env file is never
// copied, even when the allow-list matches it, and the copied files' content
// goes through the secret scanner first - a credential shape in one of them
// refuses the whole run before any target is written. Repositories are carried by
// push, not here; scheduling the job on the hub's tick is its caller's.
package backup

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/tools/secret-scan/secretscan"
)

// DefaultConfigAllow is the allow-list of configuration files of a universe's
// folder: its top-level YAML files, never a subfolder (sessions, keys).
var DefaultConfigAllow = []string{"*.yaml", "*.yml"}

// Keep is how many generations pruning keeps.
const (
	KeepHourly = 24
	KeepDaily  = 14
)

// Job is one run of the backup of one universe.
type Job struct {
	Universe  string    // the universe's name: the copies' prefix
	ConfigDir string    // the universe's folder the allow-list reads
	Allow     []string  // glob patterns relative to ConfigDir; DefaultConfigAllow when empty
	Snapshot  string    // the scheduler store's newest snapshot; none when empty
	Local     string    // the local target (every run); required
	OffPC     string    // the off-PC target (the day's first run); none when empty
	LocalOnly bool      // the universe never goes off the PC
	Now       time.Time // the run's time (UTC stamp)
}

// Result names the files a run wrote.
type Result struct {
	Written []string // archives written (local and off-PC), each beside its .sha256
	OffPC   bool     // the run wrote to the off-PC target
	Pruned  []string // archives removed by pruning
}

const stampLayout = "20060102T150405Z"

// IsSecretFile answers whether a file name is a credentials file the job never
// copies: .env and .env.<purpose>, except a committed .example.
func IsSecretFile(name string) bool {
	base := filepath.Base(name)
	if base != ".env" && !strings.HasPrefix(base, ".env.") {
		return false
	}
	return !strings.HasSuffix(base, ".example")
}

// Run copies the job's files to its targets and prunes old generations.
func Run(j Job) (Result, error) {
	var res Result
	if j.Universe == "" || j.Local == "" {
		return res, errors.New("backup: a universe and a local target are required")
	}
	now := j.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	stamp := now.Format(stampLayout)
	targets := []string{j.Local}
	if j.OffPC != "" && !j.LocalOnly {
		first, err := firstOfDay(j.OffPC, j.Universe, now)
		if err != nil {
			return res, err
		}
		if first {
			targets = append(targets, j.OffPC)
			res.OffPC = true
		}
	}
	names, err := configFiles(j.ConfigDir, j.Allow)
	if err != nil {
		return res, err
	}
	files, err := readConfig(j.ConfigDir, names)
	if err != nil {
		return res, err
	}
	if err := refuseSecrets(files); err != nil {
		return res, err
	}
	for _, dir := range targets {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return res, fmt.Errorf("backup: target %s: %w", dir, err)
		}
		if j.Snapshot != "" {
			p := filepath.Join(dir, j.Universe+"-"+stamp+".db.gz")
			if err := seal(p, func(w io.Writer) error { return gzipFile(w, j.Snapshot) }); err != nil {
				return res, err
			}
			res.Written = append(res.Written, p)
		}
		if len(names) > 0 {
			p := filepath.Join(dir, j.Universe+"-"+stamp+"-config.tar.gz")
			if err := seal(p, func(w io.Writer) error { return tarFiles(w, names, files) }); err != nil {
				return res, err
			}
			res.Written = append(res.Written, p)
		}
		pruned, err := Prune(dir, j.Universe, now)
		if err != nil {
			return res, err
		}
		res.Pruned = append(res.Pruned, pruned...)
	}
	return res, nil
}

// configFiles answers the allow-listed regular files of dir, relative and
// sorted, never a credentials file.
func configFiles(dir string, allow []string) ([]string, error) {
	if dir == "" {
		return nil, nil
	}
	if len(allow) == 0 {
		allow = DefaultConfigAllow
	}
	seen := map[string]bool{}
	var out []string
	for _, pat := range allow {
		matches, err := filepath.Glob(filepath.Join(dir, pat))
		if err != nil {
			return nil, fmt.Errorf("backup: allow-list pattern %q: %w", pat, err)
		}
		for _, m := range matches {
			rel, err := filepath.Rel(dir, m)
			if err != nil || seen[rel] || IsSecretFile(rel) {
				continue
			}
			if st, err := os.Lstat(m); err != nil || !st.Mode().IsRegular() {
				continue
			}
			seen[rel] = true
			out = append(out, rel)
		}
	}
	sort.Strings(out)
	return out, nil
}

// readConfig reads the allow-listed files once: what the scanner checks is
// exactly what the archive carries.
func readConfig(dir string, names []string) (map[string][]byte, error) {
	files := make(map[string][]byte, len(names))
	for _, rel := range names {
		b, err := os.ReadFile(filepath.Join(dir, rel))
		if err != nil {
			return nil, fmt.Errorf("backup: %w", err)
		}
		files[filepath.ToSlash(rel)] = b
	}
	return files, nil
}

// refuseSecrets runs the secret scanner over the files to copy and answers an
// error naming each finding by path, line and kind - never the value.
func refuseSecrets(files map[string][]byte) error {
	if len(files) == 0 {
		return nil
	}
	findings := secretscan.ScanFiles(files, secretscan.NewShapes())
	if len(findings) == 0 {
		return nil
	}
	lines := make([]string, len(findings))
	for i, f := range findings {
		lines[i] = f.String()
	}
	return fmt.Errorf("backup: refused, a configuration file carries a secret: %s", strings.Join(lines, "; "))
}

// seal writes an archive under a temporary name and its .sha256 under one,
// reads the archive back against the sum, then renames the archive and only
// after it the sum into place: a failure at any point removes both parts and
// leaves no sum without its archive.
func seal(path string, write func(io.Writer) error) error {
	part, sumPart := path+".part", path+".sha256.part"
	fail := func(err error) error {
		_ = os.Remove(part)
		_ = os.Remove(sumPart)
		return err
	}
	f, err := os.Create(part)
	if err != nil {
		return fmt.Errorf("backup: %w", err)
	}
	h := sha256.New()
	werr := write(io.MultiWriter(f, h))
	cerr := f.Close()
	if werr != nil || cerr != nil {
		return fail(fmt.Errorf("backup: writing %s: %w", path, errors.Join(werr, cerr)))
	}
	sum := hex.EncodeToString(h.Sum(nil))
	if got, err := fileSum(part); err != nil || got != sum {
		return fail(fmt.Errorf("backup: %s does not read back to its sha256", path))
	}
	line := sum + "  " + filepath.Base(path) + "\n"
	if err := os.WriteFile(sumPart, []byte(line), 0o644); err != nil {
		return fail(fmt.Errorf("backup: %w", err))
	}
	if err := os.Rename(part, path); err != nil {
		return fail(fmt.Errorf("backup: %w", err))
	}
	if err := os.Rename(sumPart, path+".sha256"); err != nil {
		_ = os.Remove(path)
		return fail(fmt.Errorf("backup: %w", err))
	}
	return nil
}

// Verify reads an archive back against its .sha256 file.
func Verify(path string) error {
	b, err := os.ReadFile(path + ".sha256")
	if err != nil {
		return err
	}
	want, _, _ := strings.Cut(strings.TrimSpace(string(b)), " ")
	got, err := fileSum(path)
	if err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("backup: %s does not match its sha256", path)
	}
	return nil
}

func fileSum(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func gzipFile(w io.Writer, src string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	zw := gzip.NewWriter(w)
	if _, err := io.Copy(zw, in); err != nil {
		return err
	}
	return zw.Close()
}

func tarFiles(w io.Writer, names []string, files map[string][]byte) error {
	zw := gzip.NewWriter(w)
	tw := tar.NewWriter(zw)
	for _, rel := range names {
		if IsSecretFile(rel) {
			return fmt.Errorf("backup: %s is a credentials file", rel)
		}
		b := files[filepath.ToSlash(rel)]
		hdr := &tar.Header{Name: filepath.ToSlash(rel), Mode: 0o644, Size: int64(len(b)), Typeflag: tar.TypeReg}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if _, err := tw.Write(b); err != nil {
			return err
		}
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return zw.Close()
}

// generation is one run's stamp in a target, with its archives.
type generation struct {
	at    time.Time
	files []string
}

func generations(dir, universe string) ([]generation, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	by := map[string]*generation{}
	prefix := universe + "-"
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasPrefix(n, prefix) || len(n) < len(prefix)+len(stampLayout) {
			continue
		}
		s := n[len(prefix) : len(prefix)+len(stampLayout)]
		at, err := time.Parse(stampLayout, s)
		if err != nil {
			continue
		}
		g := by[s]
		if g == nil {
			g = &generation{at: at}
			by[s] = g
		}
		g.files = append(g.files, filepath.Join(dir, n))
	}
	out := make([]generation, 0, len(by))
	for _, g := range by {
		out = append(out, *g)
	}
	sort.Slice(out, func(a, b int) bool { return out[a].at.After(out[b].at) })
	return out, nil
}

func firstOfDay(dir, universe string, now time.Time) (bool, error) {
	gens, err := generations(dir, universe)
	if err != nil {
		return false, err
	}
	y, m, d := now.Date()
	for _, g := range gens {
		if gy, gm, gd := g.at.Date(); gy == y && gm == m && gd == d {
			return false, nil
		}
	}
	return true, nil
}

// Prune keeps, of a universe's generations in dir, every one of the last
// KeepHourly hours and the newest of each of the last KeepDaily days, and
// removes the others' files.
func Prune(dir, universe string, now time.Time) ([]string, error) {
	gens, err := generations(dir, universe)
	if err != nil {
		return nil, err
	}
	keepDay := map[string]bool{}
	var removed []string
	for _, g := range gens { // newest first
		day := g.at.Format("20060102")
		keep := now.Sub(g.at) < KeepHourly*time.Hour
		if !keepDay[day] && now.Sub(g.at) < KeepDaily*24*time.Hour {
			keepDay[day] = true
			keep = true
		}
		if keep {
			continue
		}
		for _, f := range g.files {
			if err := os.Remove(f); err != nil && !os.IsNotExist(err) {
				return removed, err
			}
			removed = append(removed, f)
		}
	}
	return removed, nil
}
