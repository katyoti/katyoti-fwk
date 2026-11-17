// SPDX-License-Identifier: Apache-2.0

package core

import (
	"archive/tar"
	"bufio"
	"bytes"
	stdctx "context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/projectfile"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// : a user release proves the whole migration chain - so an end user
// goes from any supported release to the latest in one migration.
// testdata/supported-releases lists the git tags of the user releases whose
// data the code must still migrate, one per line, append only; each tag's
// corpus (its project.cwm.yaml and modules/) is read from git, migrated by the
// code's steps and validated as the corpus is. A step removed from the middle
// of the chain fails it.

const supportedReleasesFile = "testdata/supported-releases"

// supportedReleases reads the tags the chain must cover.
func supportedReleases(t *testing.T) []string {
	t.Helper()
	f, err := os.Open(supportedReleasesFile)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var tags []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if l := strings.TrimSpace(sc.Text()); l != "" && !strings.HasPrefix(l, "#") {
			tags = append(tags, l)
		}
	}
	return tags
}

// corpusAtTag writes the corpus of repo at tag - its project object and its
// modules folder - into a new folder, from git.
func corpusAtTag(t *testing.T, repo, tag string) string {
	t.Helper()
	modules := projectfile.DefaultModulesDir
	if out, err := exec.Command("git", "-C", repo, "show", tag+":"+projectfile.FileName).Output(); err == nil {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, projectfile.FileName), out, 0o644); err != nil {
			t.Fatal(err)
		}
		if f, err := projectfile.Read(dir); err == nil {
			modules = f.ModulesDir
		}
	}
	cmd := exec.Command("git", "-C", repo, "archive", "--format=tar", tag, "--", projectfile.FileName, modules)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git archive %s: %v %s", tag, err, stderr.String())
	}
	dst := t.TempDir()
	tr := tar.NewReader(bytes.NewReader(out))
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if h.Typeflag != tar.TypeReg {
			continue
		}
		p := filepath.Join(dst, filepath.FromSlash(h.Name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(tr)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dst
}

// TestEverySupportedReleaseMigrates: each supported release's corpus,
// migrated by the code's steps, is valid as the corpus is.
func TestEverySupportedReleaseMigrates(t *testing.T) {
	tags := supportedReleases(t)
	if len(tags) == 0 {
		t.Skip("no supported user release yet: the first user release with a data version is added to " + supportedReleasesFile)
	}
	cfg, up := realCorpusConfig(t)
	for _, tag := range tags {
		t.Run(tag, func(t *testing.T) {
			root := migratedCorpus(t, corpusAtTag(t, up, tag), migrationSteps)
			c := *cfg
			c.WorkspaceRoot = freshWorkspace(t)
			c.ProjectRoot = root
			svc, err := New(stdctx.Background(), &c)
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			defer svc.Close()
			out, err := svc.ObjectScan(stdctx.Background(), api.ObjectScanInput{Force: true, Validate: true})
			if err != nil {
				t.Fatal(err)
			}
			if out.Checked == 0 {
				t.Fatalf("release %s: no object checked", tag)
			}
			for _, inv := range out.Invalid {
				if p := shapeProblems(inv.Problems); len(p) > 0 {
					t.Errorf("release %s, migrated: %s (%s): %s", tag, inv.ID, inv.Path, strings.Join(p, " || "))
				}
			}
		})
	}
}

// TestTheChainProvesEveryStep: the check on a release of its own - a corpus
// tagged at the baseline in a scratch repository, migrated by two steps, is in
// the new shape; with the middle step removed it is not, and the check says so.
func TestTheChainProvesEveryStep(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on PATH")
	}
	_, project := migrationFixture(t)
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", project, "-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	git("init", "-q")
	git("add", "-A")
	git("commit", "-q", "-m", "release 1")
	git("tag", "r1")

	inShape := func(steps []MigrationStep) error {
		dir := migratedCorpus(t, corpusAtTag(t, project, "r1"), steps)
		o, err := ReadObjectFile(filepath.Join(dir, api.DefaultModulesDir, "m", ObjectsDirName, "m.F-001"+ObjectFileSuffix))
		if err != nil {
			return err
		}
		want, _ := o.Capability("want")
		if want["reachability"] != "closable" || want["closable"] != true {
			return fmt.Errorf("release r1 migrated is not in the new shape: want = %v", want)
		}
		return nil
	}
	if err := inShape(twoSteps()); err != nil {
		t.Fatalf("the whole chain: %v", err)
	}
	broken := []MigrationStep{{Title: "the first step, removed", Object: func(*api.Object) (bool, error) { return false, nil }}, twoSteps()[1]}
	if err := inShape(broken); err == nil {
		t.Fatal("a chain with its first step removed still proved the release")
	}
}
