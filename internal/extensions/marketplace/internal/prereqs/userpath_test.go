// SPDX-License-Identifier: Apache-2.0

package prereqs

import (
	"strings"
	"testing"
)

// A program the engine placed goes first on the user's PATH, ahead of the
// Windows Store's python.exe stub in WindowsApps (on every user's PATH): the
// checks, pip and every program started later find it, not the stub. Already
// there - as written or through a variable - it is moved first, never doubled.
func TestAPlacedProgramComesFirstOnTheUserPath(t *testing.T) {
	stub := `%USERPROFILE%\AppData\Local\Microsoft\WindowsApps`
	tools := `C:\Users\me\.katyoti\tools\python\tools`
	expand := func(p string) string { return strings.ReplaceAll(p, "%USERPROFILE%", `C:\Users\me`) }

	got := userPathWith(stub+`;D:\other`, tools, expand)
	if want := tools + ";" + stub + `;D:\other`; got != want {
		t.Errorf("new entry: %q, want %q", got, want)
	}
	got = userPathWith(stub+";"+tools, tools, expand)
	if want := tools + ";" + stub; got != want {
		t.Errorf("entry already last: %q, want it moved first %q", got, want)
	}
	viaVar := `%USERPROFILE%\.katyoti\tools\python\tools`
	got = userPathWith(stub+";"+viaVar, tools, expand)
	if strings.Count(strings.ToLower(got), `.katyoti\tools\python\tools`) != 1 || !strings.HasPrefix(got, tools+";") {
		t.Errorf("entry through a variable: %q, want it once, first", got)
	}
	if got := userPathWith("", tools, expand); got != tools {
		t.Errorf("empty PATH: %q, want %q", got, tools)
	}
}
