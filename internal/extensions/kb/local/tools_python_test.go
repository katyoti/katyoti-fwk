// SPDX-License-Identifier: Apache-2.0

package local

import "testing"

// : the PDF packages pip installs are declared as Python
// packages, so « Prêt à démarrer » finds them where pip put them, never as a
// command on PATH.
func TestThePipPackagesAreCheckedAsPythonPackages(t *testing.T) {
	want := map[string]bool{"pymupdf": true, "pymupdf4llm": true}
	for _, d := range ToolDefs() {
		if !want[d.Name] {
			continue
		}
		delete(want, d.Name)
		if d.PythonPackage != d.Name || len(d.Commands) > 0 {
			t.Errorf("%s: PythonPackage %q, Commands %v - want the package %s and no command", d.Name, d.PythonPackage, d.Commands, d.Name)
		}
	}
	for name := range want {
		t.Errorf("%s is not declared", name)
	}
}
