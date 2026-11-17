// SPDX-License-Identifier: Apache-2.0

package publish

import (
	"os"
	"os/exec"
	"strings"
	"sync"
)

// PythonCommand is the Python this machine runs pip with: the first of
// python3, python and py on PATH that answers --version as Python - the
// Windows Store's python.exe stub answers with words and is skipped. "" when
// there is none. Installing a pip recipe (`<python> -m pip`) and checking a
// Python package both take this one, so a package is looked for in the
// Python it was installed into.
//
// A Python found is remembered for the PATH it was found on: a PATH read
// again (a Python just installed) looks again.
func PythonCommand() string {
	path := os.Getenv("PATH")
	pythonMu.Lock()
	defer pythonMu.Unlock()
	if found, ok := pythonFound[path]; ok {
		return found
	}
	for _, name := range []string{"python3", "python", "py"} {
		p, err := exec.LookPath(name)
		if err != nil {
			continue
		}
		if ver, _ := CheckToolVersion(p, "--version"); ver != "" && ver[0] >= '0' && ver[0] <= '9' {
			pythonFound[path] = name
			return name
		}
	}
	return ""
}

var (
	pythonMu    sync.Mutex
	pythonFound = map[string]string{}
)

// pythonPackageVersion is the Python snippet printing an installed
// distribution's version from its metadata - read, never imported: importing
// a package can be slow or fail on a dependency while the package is there.
const pythonPackageVersion = "import sys, importlib.metadata as m; print(m.version(sys.argv[1]))"

// CheckPythonPackage checks a Python package (a pip recipe's package) as a
// package: its version as the Python pip installs with (PythonCommand) knows
// it, wherever pip put it (user or system site). A package is not a command:
// pip leaves its scripts, when it has any, in a folder no PATH holds.
func CheckPythonPackage(pkg string) CheckResult {
	py := PythonCommand()
	if py == "" {
		return CheckResult{AdditionalInfo: "no Python on this machine to hold the package " + pkg}
	}
	cmd := exec.Command(py, "-c", pythonPackageVersion, pkg)
	NoConsoleWindow(cmd)
	out, err := cmd.Output()
	ver := strings.TrimSpace(string(out))
	if err != nil || ver == "" {
		return CheckResult{AdditionalInfo: "the Python package " + pkg + " is not installed for " + py}
	}
	return CheckResult{OK: true, FoundVersion: ver}
}
