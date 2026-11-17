// SPDX-License-Identifier: Apache-2.0

package cenv

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

// projects.yaml holds bare paths (untrusted) and {path, trusted} entries; the
// list of projects reads both, ProjectTrusted answers the operator's word, and
// an entry is written back as it was read.
func TestAttachedProjectsCarryTheOperatorsTrust(t *testing.T) {
	root := t.TempDir()
	a, b := filepath.Join(root, "a"), filepath.Join(root, "b")
	body := "projects:\n  - " + a + "\n  - path: " + b + "\n    trusted: true\n"
	if err := os.WriteFile(filepath.Join(root, AttachedProjectsFile), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	got := AttachedProjects(root)
	if len(got) != 2 || got[0] != a || got[1] != b {
		t.Fatalf("projects = %v", got)
	}
	if ProjectTrusted(root, a) || !ProjectTrusted(root, b) || ProjectTrusted(root, filepath.Join(root, "c")) {
		t.Fatalf("trusted: a=%v b=%v c=%v; want false true false", ProjectTrusted(root, a), ProjectTrusted(root, b), ProjectTrusted(root, filepath.Join(root, "c")))
	}
	out, err := yaml.Marshal(struct {
		Projects []AttachedEntry `yaml:"projects"`
	}{[]AttachedEntry{{Path: a}, {Path: b, Trusted: true}}})
	if err != nil {
		t.Fatal(err)
	}
	var back struct {
		Projects []AttachedEntry `yaml:"projects"`
	}
	if err := yaml.Unmarshal(out, &back); err != nil || len(back.Projects) != 2 || back.Projects[0].Trusted || !back.Projects[1].Trusted {
		t.Fatalf("round trip = %+v (%v) from\n%s", back.Projects, err, out)
	}
}
