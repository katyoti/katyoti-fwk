// SPDX-License-Identifier: Apache-2.0

package projectfile

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeProject(t *testing.T, body string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "My_Project")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if body != "" {
		if err := os.WriteFile(filepath.Join(dir, FileName), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// TestAProjectWithoutTheFileReadsTheDefaults: no project object is not a
// fault; the folder's name stands for the id and every default applies.
func TestAProjectWithoutTheFileReadsTheDefaults(t *testing.T) {
	f, err := Read(writeProject(t, ""))
	if err != nil {
		t.Fatal(err)
	}
	want := File{ID: "My_Project", Name: "My_Project", ModulesDir: "modules", Contributions: Contributions{Provider: "my-project", Root: "modules/sys-contributions"},
		DataVersion: BaselineDataVersion}
	if !reflect.DeepEqual(f, want) {
		t.Fatalf("got %+v, want %+v", f, want)
	}
}

// TestTheDeclaredValuesAreRead: what the file declares wins over every
// default, and the contributions root follows a declared modules folder.
func TestTheDeclaredValuesAreRead(t *testing.T) {
	f, err := Read(writeProject(t, `id: acme
type: project
capabilities:
  project:
    uuid: 00000000-0000-7000-8000-000000000001
    modules_dir: docs/modules
    data_version: 4
`))
	if err != nil {
		t.Fatal(err)
	}
	want := File{ID: "acme", Type: "project", Name: "acme", UUID: "00000000-0000-7000-8000-000000000001", ModulesDir: "docs/modules",
		Contributions: Contributions{Provider: "acme", Root: "docs/modules/sys-contributions"}, DataVersion: 4}
	if !reflect.DeepEqual(f, want) {
		t.Fatalf("got %+v, want %+v", f, want)
	}

	f, err = Read(writeProject(t, `id: acme
capabilities:
  contributions:
    provider: acme-pack
    root: packs/acme
`))
	if err != nil {
		t.Fatal(err)
	}
	if f.Contributions != (Contributions{Provider: "acme-pack", Root: "packs/acme"}) {
		t.Fatalf("declared contributions not read: %+v", f.Contributions)
	}
}

// TestTheScanExclusionsAreRead: `scan.exclude` reaches the reader as written,
// and a glob doublestar cannot parse is refused, naming it.
func TestTheScanExclusionsAreRead(t *testing.T) {
	f, err := Read(writeProject(t, `id: acme
capabilities:
  project:
    scan:
      exclude: ["modules/*/drafts/**", "modules/a/objects/old.cwm.yaml"]
`))
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"modules/*/drafts/**", "modules/a/objects/old.cwm.yaml"}; !reflect.DeepEqual(f.ScanExclude, want) {
		t.Fatalf("scan exclude = %v, want %v", f.ScanExclude, want)
	}
	for _, g := range []string{"modules/[a", "modules/{a"} {
		_, err := Read(writeProject(t, "id: acme\ncapabilities:\n  project:\n    scan:\n      exclude: [\""+g+"\"]\n"))
		if err == nil || !strings.Contains(err.Error(), g) {
			t.Errorf("invalid glob %q: err = %v, want a refusal naming it", g, err)
		}
	}
}

// TestANonKebabProviderIsRefused: a provider is a token in URIs and folder
// names; anything but kebab is refused rather than published.
func TestANonKebabProviderIsRefused(t *testing.T) {
	for _, p := range []string{"Acme", "acme_pack", "acme pack", "-acme"} {
		dir := writeProject(t, "id: acme\ncapabilities:\n  contributions:\n    provider: \""+p+"\"\n")
		if _, err := Read(dir); err == nil {
			t.Errorf("provider %q was accepted", p)
		}
	}
}

// TestTheRepositorysProjectFileReads: the real project object reads without
// error and names its modules folder.
func TestTheRepositorysProjectFileReads(t *testing.T) {
	root, _ := filepath.Abs(filepath.Join("..", "..", ".."))
	if _, err := os.Stat(filepath.Join(root, FileName)); err != nil {
		t.Skip("no project object at the repository root")
	}
	f, err := Read(root)
	if err != nil {
		t.Fatal(err)
	}
	if f.ID != "ktw-agents" || f.UUID == "" || f.ModulesDir != "modules" {
		t.Fatalf("unexpected project: %+v", f)
	}
}

// TestADataVersionBelowTheFirstIsRefused: versions start at the baseline.
func TestADataVersionBelowTheFirstIsRefused(t *testing.T) {
	if _, err := Read(writeProject(t, `id: acme
capabilities:
  project:
    data_version: -1
`)); err == nil {
		t.Fatal("a data version below the baseline was read")
	}
}
