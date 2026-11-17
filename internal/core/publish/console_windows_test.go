// SPDX-License-Identifier: Apache-2.0

//go:build windows

package publish

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
)

// consoleProbeEnv makes the test binary a probe instead of a test run:
// "child" prints its own console state; "parent" prints its own, then starts
// itself as "child" plainly — no flags, as any code does — and relays it.
const consoleProbeEnv = "KTW_PUBLISH_CONSOLE_PROBE"

var (
	getConsoleCP     = syscall.NewLazyDLL("kernel32.dll").NewProc("GetConsoleCP")
	getConsoleWindow = syscall.NewLazyDLL("kernel32.dll").NewProc("GetConsoleWindow")
	isWindowVisible  = syscall.NewLazyDLL("user32.dll").NewProc("IsWindowVisible")
)

// consoleState is "none" (no console: every console program it starts gets a
// window of its own), "visible" (a console window on screen) or "hidden" (a
// console with no window, or with one never shown).
func consoleState() string {
	if cp, _, _ := getConsoleCP.Call(); cp == 0 {
		return "none"
	}
	if hwnd, _, _ := getConsoleWindow.Call(); hwnd != 0 {
		if v, _, _ := isWindowVisible.Call(hwnd); v != 0 {
			return "visible"
		}
	}
	return "hidden"
}

func init() {
	switch os.Getenv(consoleProbeEnv) {
	case "child":
		fmt.Println("child=" + consoleState())
		os.Exit(0)
	case "parent":
		fmt.Println("parent=" + consoleState())
		c := exec.Command(os.Args[0])
		c.Env = append(os.Environ(), consoleProbeEnv+"=child")
		out, err := c.Output()
		if err != nil {
			fmt.Println("child-error=" + err.Error())
		}
		fmt.Print(string(out))
		os.Exit(0)
	}
}

// TestADetachedProcessOpensNoWindowDownItsTree: a process started with Detach
// (a hub kept running, a scenario case's hub) has a console of its own, hidden,
// and a console program it starts with a bare exec.Command — the manifest
// probe of each member, a version probe — inherits it: no window anywhere. A
// process with no console at all (DETACHED_PROCESS) failed both.
func TestADetachedProcessOpensNoWindowDownItsTree(t *testing.T) {
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), consoleProbeEnv+"=parent")
	Detach(cmd)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	got := strings.Fields(string(out))
	want := []string{"parent=hidden", "child=hidden"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("console states = %v, want %v", got, want)
	}
}

// TestNoConsoleWindowHidesTheChildsConsole: a console program started with
// NoConsoleWindow gets a console it can use, never shown.
func TestNoConsoleWindowHidesTheChildsConsole(t *testing.T) {
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), consoleProbeEnv+"=child")
	NoConsoleWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if got := strings.TrimSpace(string(out)); got != "child=hidden" {
		t.Fatalf("console state = %q, want child=hidden", got)
	}
}
