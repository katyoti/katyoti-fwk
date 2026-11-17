// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"sort"
	"sync"
	"testing"

	projection "github.com/katyoti/katyoti-fwk/internal/extensions/code/cwmprojection"
	"github.com/katyoti/katyoti-fwk/internal/extensions/gocode/api"
)

// dirTools is a GoToolService that records the folder each tool is asked to
// run in.
type dirTools struct {
	mu   sync.Mutex
	dirs map[string]string
}

func (d *dirTools) saw(tool, dir string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.dirs[tool] = dir
}

func (d *dirTools) GoBuild(_ context.Context, in api.GoBuildInput) (api.GoBuildOutput, error) {
	d.saw("build", in.WorkDir)
	return api.GoBuildOutput{}, nil
}
func (d *dirTools) GoTest(_ context.Context, in api.GoTestInput) (api.GoTestOutput, error) {
	d.saw("test", in.WorkDir)
	return api.GoTestOutput{}, nil
}
func (d *dirTools) GoVet(_ context.Context, in api.GoVetInput) (api.GoVetOutput, error) {
	d.saw("vet", in.WorkDir)
	return api.GoVetOutput{}, nil
}
func (d *dirTools) GolangciLint(context.Context, api.GolangciLintInput) (api.GolangciLintOutput, error) {
	return api.GolangciLintOutput{}, nil
}
func (d *dirTools) GoAPIDiff(context.Context, api.GoAPIDiffInput) (api.GoAPIDiffOutput, error) {
	return api.GoAPIDiffOutput{}, nil
}
func (d *dirTools) GoPkgInventory(_ context.Context, in api.GoPkgInventoryInput) (api.GoPkgInventoryOutput, error) {
	d.saw("inventory", in.WorkDir)
	return api.GoPkgInventoryOutput{}, nil
}
func (d *dirTools) GoDepCheck(_ context.Context, in api.GoDepCheckInput) (api.GoDepCheckOutput, error) {
	d.saw("depcheck", in.WorkDir)
	return api.GoDepCheckOutput{}, nil
}
func (d *dirTools) Gofmt(context.Context, api.GofmtInput) (api.GofmtOutput, error) {
	return api.GofmtOutput{}, nil
}
func (d *dirTools) Goimports(context.Context, api.GoimportsInput) (api.GoimportsOutput, error) {
	return api.GoimportsOutput{}, nil
}
func (d *dirTools) GoModTidy(context.Context, api.GoModTidyInput) (api.GoModTidyOutput, error) {
	return api.GoModTidyOutput{}, nil
}

// The go checks run every tool in the module's project root, whatever folder
// the process runs in; with no root known they run nothing and say why.
func TestTheGoChecksRunInTheProjectRoot(t *testing.T) {
	tools := &dirTools{dirs: map[string]string{}}
	root := t.TempDir()
	sc := checkScope{roots: []string{"./internal/x"}, patterns: []string{"./internal/x/..."}, dir: root}
	collectAuditFindings(context.Background(), tools, projection.ManifestInputs{}, sc)
	var seen []string
	for tool, dir := range tools.dirs {
		seen = append(seen, tool)
		if dir != root {
			t.Errorf("%s ran in %q, want the project root %s", tool, dir, root)
		}
	}
	sort.Strings(seen)
	if want := []string{"build", "depcheck", "inventory", "test", "vet"}; len(seen) != len(want) {
		t.Fatalf("tools run %v, want %v", seen, want)
	}

	none := &dirTools{dirs: map[string]string{}}
	got := collectAuditFindings(context.Background(), none, projection.ManifestInputs{}, checkScope{patterns: sc.patterns, roots: sc.roots})
	if len(none.dirs) != 0 || len(got) != 1 || got[0].FindingID != "correctness-root-1" {
		t.Fatalf("no root: tools %v, findings %+v; want none run and the one root finding", none.dirs, got)
	}
}
