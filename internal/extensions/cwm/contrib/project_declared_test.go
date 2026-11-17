// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"io/fs"
	"os"
	"strings"
	"testing"
)

// TestCWMDeclaresTheProject: the `project` capability - the project level -
// and its data-type are CWM's own declarations (CorePack), so every binary
// that hosts CWM reads the project object without the method pack; the bare
// `project` type is retired: a project is of a project type
// (software-project, a domain's) and an unconverted
// repository's is read in its legacy shape.
func TestCWMDeclaresTheProject(t *testing.T) {
	for _, f := range []string{
		"embed/cwm/capabilities/project.yaml",
		"embed/cwm/data-types/cap-project.dsl.yaml",
	} {
		if _, err := fs.Stat(corePackFS, f); err != nil {
			t.Errorf("CWM does not declare %s: %v", f, err)
		}
	}
	if _, err := fs.Stat(corePackFS, "embed/cwm/object-types/project.yaml"); err == nil {
		t.Error("CWM ships the retired project type again (object-types/project.yaml)")
	}
	capability, err := fs.ReadFile(corePackFS, "embed/cwm/capabilities/project.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(capability), "schema: ktw-agents:/data-type/cap-project") {
		t.Error("the project capability does not name CWM's cap-project")
	}
	for _, f := range []string{
		"../../../../app/ktw-methods/embed/cwm/object-types/project.yaml",
		"../../../../app/ktw-methods/embed/cwm/capabilities/project.yaml",
		"../../../../app/ktw-methods/embed/cwm/data-types/cap-project.dsl.yaml",
	} {
		if _, err := os.Stat(f); err == nil {
			t.Errorf("the method pack still declares %s", f)
		}
	}
}
