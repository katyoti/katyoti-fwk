// SPDX-License-Identifier: Apache-2.0

package appcore

import (
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/debug"
	"strconv"
	"testing"
	"time"
)

// keepMemoryLimit restores the process's memory limit after a test changes it.
func keepMemoryLimit(t *testing.T) {
	prev := debug.SetMemoryLimit(-1)
	t.Cleanup(func() { debug.SetMemoryLimit(prev) })
}

// : a program started without GOMEMLIMIT runs under the default
// soft limit - the hub reached 4 GB of garbage without one.
func TestAProgramWithoutGOMEMLIMITRunsUnderTheDefaultLimit(t *testing.T) {
	keepMemoryLimit(t)
	t.Setenv("GOMEMLIMIT", "")
	debug.SetMemoryLimit(1 << 62)
	if got := ApplyRuntimeGuards("probe"); got != DefaultMemoryLimit {
		t.Fatalf("ApplyRuntimeGuards answered %d, want %d", got, DefaultMemoryLimit)
	}
	if got := debug.SetMemoryLimit(-1); got != DefaultMemoryLimit {
		t.Fatalf("memory limit in force %d, want %d", got, DefaultMemoryLimit)
	}
}

// review: ktw-agents holds 425-650 MB live on a real workspace
// under a 512 MiB soft limit its collector ran every ~200 ms (3.3x CPU, a
// quarter fewer commands). It runs without a limit, or one of at least
// 1.5 GiB, until measures it; the hub and the small sidecars keep
// the default, above their live heap.
func TestKtwAgentsIsNotHeldUnderItsLiveHeap(t *testing.T) {
	keepMemoryLimit(t)
	t.Setenv("GOMEMLIMIT", "")
	const floor int64 = 3 << 29 // 1.5 GiB
	if got := MemoryLimitOf("ktw-agents"); got != NoMemoryLimit && got < floor {
		t.Fatalf("ktw-agents' soft memory limit is %d MiB: below 1.5 GiB its live heap makes the collector run nearly continuously", got>>20)
	}
	debug.SetMemoryLimit(DefaultMemoryLimit)
	if got := ApplyRuntimeGuards("ktw-agents"); got != MemoryLimitOf("ktw-agents") || debug.SetMemoryLimit(-1) != got {
		t.Fatalf("ApplyRuntimeGuards(ktw-agents) left %d in force, want %d", debug.SetMemoryLimit(-1), MemoryLimitOf("ktw-agents"))
	}
	for _, name := range []string{"hub", "ktw-methods"} {
		if got := MemoryLimitOf(name); got != DefaultMemoryLimit {
			t.Errorf("MemoryLimitOf(%q) = %d, want the default %d", name, got, DefaultMemoryLimit)
		}
	}
}

// GOMEMLIMIT, when set, is the person's: the guard leaves the limit the
// runtime read from it.
func TestGOMEMLIMITWinsOverTheDefault(t *testing.T) {
	keepMemoryLimit(t)
	t.Setenv("GOMEMLIMIT", "2GiB")
	debug.SetMemoryLimit(2 << 30)
	if got := ApplyRuntimeGuards("probe"); got != 2<<30 {
		t.Fatalf("ApplyRuntimeGuards answered %d, want the GOMEMLIMIT one %d", got, int64(2<<30))
	}
}

// Every long-lived program applies the guards at its start: the sidecars and
// the hub through BootstrapCLI, ktw-agents in its own Execute. Removing the
// call from either fails here.
func TestEveryProgramStartAppliesTheRuntimeGuards(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	here := filepath.Dir(file)
	repo := filepath.Join(here, "..", "..")
	for _, c := range []struct{ path, fn, call string }{
		{filepath.Join(here, "app_ext.go"), "BootstrapCLI", `ApplyRuntimeGuards(name)`},
		{filepath.Join(repo, "app", "ktw-agents", "cmd", "root.go"), "Execute", `appcore.ApplyRuntimeGuards("ktw-agents")`},
	} {
		src, err := os.ReadFile(c.path)
		if err != nil {
			t.Fatal(err)
		}
		body := regexp.MustCompile(`(?s)func ` + c.fn + `\(.*?\r?\n}\r?\n`).Find(src)
		if body == nil {
			t.Fatalf("%s: func %s not found", c.path, c.fn)
		}
		if !regexp.MustCompile(regexp.QuoteMeta(c.call)).Match(body) {
			t.Errorf("%s: %s does not call %s - the program starts without its memory limit", c.path, c.fn, c.call)
		}
	}
	hub, err := os.ReadFile(filepath.Join(repo, "app", "ktw-hub", "cmd", "root.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`appcore\.BootstrapCLI\("hub"`).Match(hub) {
		t.Errorf("the hub no longer starts through BootstrapCLI(\"hub\"): its guards (and KTW_PPROF_HUB) are not applied")
	}
}

func TestPprofEnvNamesTheProgram(t *testing.T) {
	for name, want := range map[string]string{"hub": "KTW_PPROF_HUB", "ktw-agents": "KTW_PPROF_KTW_AGENTS"} {
		if got := PprofEnv(name); got != want {
			t.Errorf("PprofEnv(%q) = %q, want %q", name, got, want)
		}
	}
}

// The diagnostics endpoint answers a heap profile on the loopback.
func TestTheDiagnosticsEndpointServesTheHeapProfile(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)
	_ = ln.Close()
	ServePprof("probe", port)
	var resp *http.Response
	for i := 0; i < 50; i++ {
		resp, err = http.Get("http://127.0.0.1:" + port + "/debug/pprof/heap?debug=1")
		if err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("heap profile: status %d", resp.StatusCode)
	}
}
