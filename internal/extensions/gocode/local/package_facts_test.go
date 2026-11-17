// SPDX-License-Identifier: Apache-2.0

package local

import (
	"os"
	"path/filepath"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	gcapi "github.com/katyoti/katyoti-fwk/internal/extensions/gocode/api"
	gccore "github.com/katyoti/katyoti-fwk/internal/extensions/gocode/internal/core"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
	"testing/fstest"
)

// The package facts service takes the project root from CWM (never from a
// folder) and lists a module's packages with their facts.
func TestPackageFactsServesTheProjectCWMServes(t *testing.T) {
	root := t.TempDir()
	ctx, cwm := cwmapp.NewAt(t, root, cwmapp.Pack(fstest.MapFS{
		"object-types/module.yaml":       {Data: []byte("name: module\ntree: node\ncapabilities: [module]\n")},
		"capabilities/module.yaml":       {Data: cwmapp.ShippedFile(t, "app/ktw-methods/embed/cwm/capabilities/module.yaml")},
		"capabilities/code.yaml":         {Data: cwmapp.ShippedFile(t, "app/ktw-code/embed/cwm/capabilities/code.yaml")},
		"data-types/cap-module.dsl.yaml": {Data: cwmapp.ShippedFile(t, "app/ktw-methods/embed/cwm/data-types/cap-module.dsl.yaml")},
	}))
	m := cwmapi.NewContainerObject("m", "m", "module", "", "m", "")
	m.Capabilities["code"] = map[string]any{"code_packages": []any{"internal/x"}}
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "m", Object: m}); err != nil {
		t.Fatal(err)
	}
	res, err := cwm.ObjectResolve(ctx, cwmapi.ObjectResolveInput{Ref: "m"})
	if err != nil || res.ProjectRoot == "" {
		t.Fatalf("project root: %+v %v", res, err)
	}
	for rel, content := range map[string]string{
		"go.mod":          "module example.com/p\n",
		"internal/x/x.go": "// Package x is X.\npackage x\n",
		"internal/y/y.go": "package y\n",
	} {
		p := filepath.Join(res.ProjectRoot, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	svc := gccore.NewPackageFactsIn(t.TempDir())
	out, err := svc.PackageList(ctx, gcapi.PackageListInput{Module: "m"})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Packages) != 1 || out.Packages[0].Path != "internal/x" || out.Packages[0].Description != "Package x is X." || out.Packages[0].Owner != "m" {
		t.Fatalf("m's packages: %+v", out.Packages)
	}
}
