// SPDX-License-Identifier: Apache-2.0

package prereqs

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/publish"
	mktapi "github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/api"
)

// A NuGet package is unpacked as the zip it is, and the folder put on PATH is
// the one holding the command at the shallowest depth: Python's package also
// carries a venv launcher stub named python.exe under Lib/venv/scripts/nt/.
func TestInstall_ANugetPackageIsUnpackedAndItsShallowestCommandOnPath(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, name := range []string{"python.nuspec", "tools/Lib/venv/scripts/nt/python.exe", "tools/Lib/site-packages/pip/__init__.py", "tools/python.exe"} {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(name))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	body := buf.Bytes()
	sum := sha256.Sum256(body)
	needs := []Need{{Sidecar: "kb", Name: "python", Required: true, Checked: true, Commands: []string{"python", "python3", "py"},
		Recipes: []publish.InstallRecipe{{Platform: "windows", Kind: publish.RecipeDownload,
			URL: "https://api.nuget.org/v3-flatcontainer/python/3.12.10/python.3.12.10.nupkg", SHA256: hex.EncodeToString(sum[:])}}}}
	tools := t.TempDir()
	f := newFake()
	e := &Engine{Runner: f, Platform: "windows", ToolsDir: tools,
		Fetch: func(context.Context, string) ([]byte, error) { return body, nil }}
	out, err := e.Install(context.Background(), e.Plan(needs), nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Steps) != 1 || out.Steps[0].Status != mktapi.StepInstalled {
		t.Fatalf("steps %+v, want python installed", out.Steps)
	}
	want := filepath.Join(tools, "python", "tools")
	if len(f.added) != 1 || filepath.Clean(f.added[0]) != want {
		t.Errorf("PATH got %v, want %s (not the venv stub's folder)", f.added, want)
	}
}
