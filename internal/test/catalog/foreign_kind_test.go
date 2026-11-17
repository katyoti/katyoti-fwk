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

// goRecipeProducerPkg is the Go fixture that publishes `recipe` entries and
// declares the kind nowhere (blind audit H3).
const goRecipeProducerPkg = "github.com/katyoti/katyoti-fwk/internal/test/fixtures/contributions/go-recipe-producer"

// buildGoRecipeProducer compiles the fixture into a temp dir, like the
// polyglot producer: it is never part of bin/.
func buildGoRecipeProducer(t *testing.T) string {
	t.Helper()
	exe := filepath.Join(t.TempDir(), "go-recipe-producer")
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	if out, err := exec.Command("go", "build", "-o", exe, goRecipeProducerPkg).CombinedOutput(); err != nil {
		t.Fatalf("build go-recipe-producer: %v\n%s", err, out)
	}
	return exe
}

// TestForeignKindThroughTheUniverse is the cross-process proof of blind audit
// H3: a Go sidecar and a hosted `dirs:` pack each publish entries of a kind
// they do not declare — `recipe`, declared with its schema by files-only-pack
// — and the universe treats them exactly like the polyglot producer's: the
// members' manifests build (the kind recorded as foreign, never a refused
// manifest), the entries are listed, opened by layer through the one consumer
// lane, and validated by the hub against the DECLARING member's schema. The
// declaring pack is listed LAST, so precedence and resolution owe nothing to
// member order.
func TestForeignKindThroughTheUniverse(t *testing.T) {
	root := repoRoot(t)
	hostBin := binaryFor(t, "ktw-host")
	if hostBin == "" || binaryFor(t, "ktw-hub") == "" {
		hubharness.MissingBinary(t, "foreign kind: bin/ktw-host or bin/ktw-hub not built")
		return
	}
	producer := buildGoRecipeProducer(t)

	const (
		goEntryURI   = "go-recipe-producer:/recipe/crepes"
		packEntryURI = "foreign-kind-pack:/recipe/waffles"
	)
	sender, cat := startPackUniverse(t, "foreign-kind-live", []packMember{
		{Name: "go-recipe-producer", Binary: producer},
		{Name: "foreign-kind-pack", Binary: hostBin, Folder: fixtureCopy(t, root, "foreign-kind-pack")},
		// Last: the member that DECLARES the kind and ships its schema.
		{Name: "files-only-pack", Binary: hostBin, Folder: fixtureCopy(t, root, "files-only-pack")},
	})

	listed := waitListed(t, sender, []string{"recipe"}, []string{fixtureEntryURI, goEntryURI, packEntryURI}, 60*time.Second)
	assertFixtureEntryReadable(t, cat, goEntryURI, "Crepes", "250 ml milk")
	assertFixtureEntryReadable(t, cat, packEntryURI, "Waffles", "400 ml milk")

	for _, uri := range []string{goEntryURI, packEntryURI} {
		entry := listed[uri]
		if entry.Valid == nil || !*entry.Valid || len(entry.ValidationErrors) > 0 {
			t.Errorf("%s: verdict valid=%v errors=%v — want an explicit pass against %s",
				uri, entry.Valid, entry.ValidationErrors, fixtureSchemaURI)
		}
		if _, ok := entry.Layers[contribapi.LayerSource]; !ok {
			t.Errorf("%s: the index lost the producer's source layer descriptor: %v", uri, entry.Layers)
		}
	}
}

// TestKindNoMemberDeclaresIsRefusedOnTheEntry is the other half of H3's
// move: the refusal of a truly undeclared kind now lives at the aggregator,
// per entry. The same Go producer in a universe where NOBODY declares
// `recipe` is still a member — its manifest built, its entry is listed — and
// that entry carries an invalid verdict naming the kind, instead of the
// member vanishing behind `undeclared contribution kind(s) [recipe]`.
func TestKindNoMemberDeclaresIsRefusedOnTheEntry(t *testing.T) {
	if binaryFor(t, "ktw-hub") == "" {
		hubharness.MissingBinary(t, "foreign kind: bin/ktw-hub not built")
		return
	}
	producer := buildGoRecipeProducer(t)
	const goEntryURI = "go-recipe-producer:/recipe/crepes"
	sender, _ := startPackUniverse(t, "undeclared-kind-live", []packMember{
		{Name: "go-recipe-producer", Binary: producer},
	})
	listed := waitListed(t, sender, []string{"recipe"}, []string{goEntryURI}, 60*time.Second)
	entry := listed[goEntryURI]
	if entry.Valid == nil || *entry.Valid {
		t.Fatalf("%s: Valid = %v, want *false: no member declares recipe", goEntryURI, entry.Valid)
	}
	if len(entry.ValidationErrors) != 1 || !strings.Contains(entry.ValidationErrors[0], `kind "recipe" is declared by no member`) {
		t.Errorf("%s: the verdict must name the undeclared kind, got %v", goEntryURI, entry.ValidationErrors)
	}
}
