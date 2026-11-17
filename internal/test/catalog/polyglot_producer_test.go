// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/test/hubharness"
)

// polyglotPkg is the fixture producer that links nothing from this repository.
const polyglotPkg = "github.com/katyoti/katyoti-fwk/internal/test/fixtures/contributions/polyglot-producer"

// TestPolyglotProducerLinksNothingFromTheRepo holds the fixture to what makes
// it a polyglot stand-in: its whole dependency graph is the standard library.
// A producer that imported the framework would prove nothing about a producer
// written in another language.
func TestPolyglotProducerLinksNothingFromTheRepo(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", "-f", "{{if not .Standard}}{{.ImportPath}}{{end}}", polyglotPkg).Output()
	if err != nil {
		t.Fatalf("go list -deps %s: %v", polyglotPkg, err)
	}
	var foreign []string
	for _, line := range strings.Fields(string(out)) {
		if line != polyglotPkg {
			foreign = append(foreign, line)
		}
	}
	if len(foreign) > 0 {
		t.Errorf("the polyglot producer links non-standard packages %v — it must speak the wire with the standard library only", foreign)
	}
}

// TestPolyglotProducerThroughTheUniverse is e19's polyglot proof: a producer
// that speaks only the member wire (discovery by `sc resources manifest`,
// content by JSON-RPC over stdio) publishes an entry of a kind another pack
// declares, and the universe treats it exactly like a framework-built entry —
// listed, opened by layer through mbapi.Open (the source layer through the
// producer's own stdio wire, since it is not resident), validated against the
// declaring pack's schema.
func TestPolyglotProducerThroughTheUniverse(t *testing.T) {
	root := repoRoot(t)
	hostBin := binaryFor(t, "ktw-host")
	if hostBin == "" || binaryFor(t, "ktw-hub") == "" {
		hubharness.MissingBinary(t, "polyglot producer: bin/ktw-host or bin/ktw-hub not built")
		return
	}
	exe := filepath.Join(t.TempDir(), "polyglot-producer")
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	if out, err := exec.Command("go", "build", "-o", exe, polyglotPkg).CombinedOutput(); err != nil {
		t.Fatalf("build polyglot producer: %v\n%s", err, out)
	}

	const entryURI = "polyglot-producer:/recipe/waffles"
	sender, cat := startPackUniverse(t, "polyglot-producer-live", []packMember{
		// First: the pack that DECLARES the
		// kind and its schema. The polyglot producer only publishes entries.
		{Name: "files-only-pack", Binary: hostBin, Folder: fixtureCopy(t, root, "files-only-pack")},
		{Name: "polyglot-producer", Binary: exe, Folder: fixtureCopy(t, root, "polyglot-producer", "content")},
	})

	listed := waitListed(t, sender, []string{"recipe"}, []string{fixtureEntryURI, entryURI}, 45*time.Second)
	assertFixtureEntryReadable(t, cat, entryURI, "Waffles", "400 ml milk")

	entry := listed[entryURI]
	if entry.Valid == nil || !*entry.Valid || len(entry.ValidationErrors) > 0 {
		t.Errorf("%s: verdict valid=%v errors=%v — want an explicit pass against %s",
			entryURI, entry.Valid, entry.ValidationErrors, fixtureSchemaURI)
	}
	if _, ok := entry.Layers[contribapi.LayerSource]; !ok {
		t.Errorf("%s: the index lost the producer's source layer descriptor: %v", entryURI, entry.Layers)
	}
}
