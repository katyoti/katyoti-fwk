// SPDX-License-Identifier: Apache-2.0

// Package binaries checks what a COMPILED SIDECAR actually publishes.
//
// # Why this exists next to the census
//
// The census asks the question in-process: it constructs the application from
// the extension list and inspects the manifest that build produces. That is
// fast and precise, and it is not the artifact anyone ships. Between the two
// sit `//go:embed` sets, build tags, init order, and whatever a `cmd/` wires
// that a test's extension list does not — every one of which can make a
// shipped binary publish something different from the code that was checked.
//
// So this asks the same question of the executable: run it, take the manifest
// it reports, and judge it by EXACTLY the rules the census applies
// (census.AssertValidWith). Two standards would be two things to keep in step,
// and the point of checking the artifact is to compare it against the same one.
//
// # What it deliberately does not do
//
// No service method is invoked. Nothing is asked of an LLM. `sc resources
// manifest` is a pure descriptor read — the binary says what it carries and
// exits — so this suite has no credentials, no network, and no per-sidecar
// knowledge. Running a sidecar's own methods is a different kind of test and
// belongs with that sidecar.
package binaries

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/test/census"
	"github.com/katyoti/katyoti-fwk/internal/test/hubharness"
	"github.com/katyoti/katyoti-fwk/internal/test/long"
)

// notSidecars are binaries with no `sc` surface, and why.
//
// A declared list with reasons rather than a sniff on the error text: a binary
// that LOSES its resources surface must fail here, and "unknown command sc"
// reads identically whether that is by design or by regression. Anything not
// named here is expected to answer, and does not get to skip.
var notSidecars = map[string]string{
	"ktw-utils": "a plain CLI tool (session/journal inspection); it is not built on sidecarkit and hosts no resource service",
	"ktw-mcp":   "the MCP server Claude Code runs over stdio: one tool per program of its installation; it hosts no resource service",
}

// manifestTimeout bounds one descriptor read. It is a spawn plus an embedded-FS
// walk; a machine that scans the executable on first run needs the headroom,
// and `make e2e-warm` pays that cost before the timed run.
const manifestTimeout = 60 * time.Second

func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, _ := runtime.Caller(0)
	return filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(thisFile))))
}

// builtBinaries returns every ktw-* / katyoti executable present in bin/.
//
// Discovered rather than listed: a sidecar added tomorrow is covered without
// anyone remembering this file, and a list that must be maintained is a list
// that silently stops covering things.
func builtBinaries(t *testing.T) map[string]string {
	t.Helper()
	binDir := filepath.Join(repoRoot(t), "bin")
	entries, err := os.ReadDir(binDir)
	if err != nil {
		hubharness.MissingBinary(t, "binaries: no bin/ directory (%v)", err)
		return nil
	}
	out := map[string]string{}
	for _, e := range entries {
		name, ok := sidecarName(e)
		if !ok {
			continue
		}
		// The test's own copy: bin/ stays replaceable while the suite runs.
		out[name] = hubharness.Binary(t, name)
	}
	if len(out) == 0 {
		hubharness.MissingBinary(t, "binaries: bin/ holds no sidecar executables")
	}
	return out
}

// sidecarName answers the sidecar a directory entry is, and whether it is one.
func sidecarName(e os.DirEntry) (string, bool) {
	if e.IsDir() {
		return "", false
	}
	// Executables only: a file beside one (ktw-mcp.allow.yaml) is not.
	if filepath.Ext(e.Name()) != exeSuffix {
		return "", false
	}
	name := strings.TrimSuffix(e.Name(), exeSuffix)
	// ktw-display is a bun-compiled TypeScript sidecar with no `sc`
	// surface; ktw-gui is a desktop app, not a sidecar.
	if name == "ktw-display" || name == "ktw-gui" || name == "ktw-test" {
		return "", false
	}
	if !strings.HasPrefix(name, "ktw-") && name != "katyoti" {
		return "", false
	}
	return name, true
}

// externalBinariesEnv names the folders of sidecars built OUTSIDE this
// repository (another repository's bin/), as a path list. Their manifests
// join the universe the census judges, by the same rules: every kind
// validates as an error, so a binary this repository does not build is
// checked before it meets a universe, not discovered there.
const externalBinariesEnv = "KTW_CENSUS_BIN_DIRS"

// externalBinaries returns the sidecars of the folders externalBinariesEnv
// names, run in place (they are not this repository's to copy). A name
// bin/ already holds stays bin/'s: a universe has one member per name, and
// another repository's copy of a binary this one builds is not its to judge.
// A named folder that does not exist fails: a census over a folder that
// is not there would check nothing and stay green.
func externalBinaries(t *testing.T, own map[string]string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, dir := range filepath.SplitList(os.Getenv(externalBinariesEnv)) {
		if dir == "" {
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("binaries: %s names %s: %v", externalBinariesEnv, dir, err)
		}
		found := 0
		for _, e := range entries {
			name, ok := sidecarName(e)
			if !ok {
				continue
			}
			found++
			if _, mine := own[name]; mine {
				continue
			}
			if prev, twice := out[name]; twice {
				t.Fatalf("binaries: %s is in two folders %s names: %s and %s", name, externalBinariesEnv, prev, filepath.Join(dir, e.Name()))
			}
			out[name] = filepath.Join(dir, e.Name())
		}
		if found == 0 {
			t.Fatalf("binaries: %s names %s, which holds no sidecar executable", externalBinariesEnv, dir)
		}
	}
	return out
}

// TestCompiledBinariesPublishSoundContributions runs every built sidecar and
// judges the manifest it reports.
func TestCompiledBinariesPublishSoundContributions(t *testing.T) {
	long.Test(t)
	bins := builtBinaries(t)
	if len(bins) == 0 {
		return
	}
	for name, path := range externalBinaries(t, bins) {
		bins[name] = path
	}

	// ONE validator for every binary: the schema extension registers into
	// package-level extension points, so a second application in this process
	// would fail on duplicate keys.
	svc := census.SchemaService(t)

	names := make([]string, 0, len(bins))
	for name := range bins {
		names = append(names, name)
	}
	sort.Strings(names)
	// The built binaries are the members of one universe: a kind one publishes
	// entries of and declares nowhere (a foreign kind) is resolved against what
	// the others declare, as the aggregator resolves it at runtime.
	manifests := map[string]*mbapi.Manifest{}
	for _, name := range names {
		if _, ok := notSidecars[name]; ok {
			continue
		}
		manifests[name] = readManifest(t, name, bins[name])
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			if why, ok := notSidecars[name]; ok {
				t.Skipf("%s publishes no contributions: %s", name, why)
			}
			m := manifests[name]
			if m.Provider == "" {
				t.Fatalf("%s reported a manifest with no provider", name)
			}
			census.AssertValidWithin(t, name, m, svc, declaredByOthers(manifests, name))
		})
	}
	t.Logf("binaries: checked %d compiled sidecar(s)", len(bins))
}

// readManifest runs `<binary> sc resources manifest --out json` and decodes it.
func readManifest(t *testing.T, name, binary string) *mbapi.Manifest {
	t.Helper()

	// An isolated, INITIALISED workspace. Isolated because pointing a sidecar
	// at the developer's real universe would let local state decide what the
	// test sees; initialised because ktw-agents refuses to start without a
	// Univers.yaml, and "not initialised" is not the answer this suite asks for.
	home := tempWorkspace(t, name)

	cmd := exec.Command(binary, "sc", "resources", "manifest", "--out", "json")
	cmd.Env = append(os.Environ(), "KTW_WORKSPACE="+home, "KTW_DATA="+home)
	var stderr strings.Builder
	cmd.Stderr = &stderr

	done := make(chan struct{})
	var out []byte
	var runErr error
	go func() {
		out, runErr = cmd.Output()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(manifestTimeout):
		_ = cmd.Process.Kill()
		t.Fatalf("%s: `sc resources manifest` did not answer within %s\n--- stderr ---\n%s",
			name, manifestTimeout, stderr.String())
	}
	if runErr != nil {
		t.Fatalf("%s: `sc resources manifest` failed: %v\n--- stderr ---\n%s",
			name, runErr, stderr.String())
	}

	var m mbapi.Manifest
	if err := json.Unmarshal(out, &m); err != nil {
		// Trimmed: a sidecar that leaks a log line onto stdout produces a
		// decode error whose message is the whole manifest otherwise.
		head := string(out)
		if len(head) > 400 {
			head = head[:400] + " …"
		}
		t.Fatalf("%s: manifest is not JSON: %v\n--- first bytes ---\n%s", name, err, head)
	}
	return &m
}

// tempWorkspace writes the minimum a sidecar needs to consider a directory a
// workspace, and returns the path to set KTW_WORKSPACE to.
func tempWorkspace(t *testing.T, name string) string {
	t.Helper()
	ws := t.TempDir()
	cfg := filepath.Join(ws, ".katyoti", "config")
	if err := os.MkdirAll(cfg, 0o755); err != nil {
		t.Fatalf("%s: mkdir workspace config: %v", name, err)
	}
	manifest := "version: 1\nname: binaries-check\ndisplay_name: Binaries Check\nicon: B\ncolor: gray\nsidecars: []\n"
	if err := os.WriteFile(filepath.Join(cfg, "Univers.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatalf("%s: write Univers.yaml: %v", name, err)
	}
	if err := os.WriteFile(filepath.Join(cfg, "Settings.yaml"), []byte("# binaries check\n"), 0o644); err != nil {
		t.Fatalf("%s: write Settings.yaml: %v", name, err)
	}
	return ws
}

// exeSuffix is an executable's suffix on this platform.
var exeSuffix = func() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}()

// declaredByOthers are the kind declarations and schemas the other members
// publish: what a member's foreign kinds resolve against.
func declaredByOthers(manifests map[string]*mbapi.Manifest, self string) []contribapi.Contribution {
	var out []contribapi.Contribution
	for name, m := range manifests {
		if name == self {
			continue
		}
		for _, c := range m.Contributions {
			if c.Type == "contribution-type" || c.Type == "data-type" {
				out = append(out, c)
			}
		}
	}
	return out
}
