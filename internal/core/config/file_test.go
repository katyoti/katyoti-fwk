// SPDX-License-Identifier: Apache-2.0

package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/variables"
)

type fileTestCfg struct {
	Name  string `yaml:"name"`
	Mode  string `yaml:"mode" default:"gui"`
	Count int    `yaml:"count,omitempty"`
}

func TestSaveAtomicThenLoadFromDirRoundTrips(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "config") // not created yet: SaveAtomic makes it
	in := fileTestCfg{Name: "x", Mode: "cli", Count: 3}
	if err := SaveAtomic(dir, "Thing", in); err != nil {
		t.Fatal(err)
	}
	if left, _ := filepath.Glob(filepath.Join(dir, "*.tmp")); len(left) > 0 {
		t.Fatalf("temp file left behind: %v", left)
	}
	var out fileTestCfg
	if err := LoadFromDir(dir, "Thing", &out); err != nil {
		t.Fatal(err)
	}
	if out != in {
		t.Fatalf("round trip: got %+v, want %+v", out, in)
	}
}

func TestLoadFromDirExpandsVariables(t *testing.T) {
	dir := t.TempDir()
	variables.RegisterResolver("filetest", func(key string) (string, bool) { return "from-" + key, true })
	if err := os.WriteFile(filepath.Join(dir, "Thing.yaml"), []byte("name: ${{filetest:resolver}}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out fileTestCfg
	if err := LoadFromDir(dir, "Thing", &out); err != nil {
		t.Fatal(err)
	}
	if out.Name != "from-resolver" || out.Mode != "gui" {
		t.Fatalf("the standard pipeline must run (expansion + default tags), got %+v", out)
	}
}

func TestLoadFromDirMissingFileKeepsDefaults(t *testing.T) {
	out := fileTestCfg{Name: "preset"}
	if err := LoadFromDir(t.TempDir(), "Absent", &out); err != nil {
		t.Fatal(err)
	}
	if out.Name != "preset" || out.Mode != "gui" {
		t.Fatalf("missing file must keep caller values and fire default tags, got %+v", out)
	}
}
