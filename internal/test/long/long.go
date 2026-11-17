// SPDX-License-Identifier: Apache-2.0

// Package long names, in one place, the tests too heavy for everyday runs
//
// Everyday runs are the quick lane: `go test -short -p 2 <packages>` with the
// test cache kept (`make test-quick`). The full lane - the final gate, the
// night - is `go test -p 2 ./... -count=1` (`make test-full`) and runs every
// test.
//
// A heavy test says so on its first line:
//
//	func TestTheRealCorpusIndexes(t *testing.T) {
//		long.Test(t)
//		...
//
// and has its row in Tests, with what it costs: the list a person reviews. A
// test calling long.Test without a row fails; this package's own test refuses
// a row no test calls and a call no row names.
package long

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Tests are the heavy tests: "<package folder>:<TestName>" -> what makes it
// heavy and about how long it takes alone (measured on the dev machine). The
// rows of tests the open-source export leaves out (private packages, packages
// not yet published, the export's own tools) are added in private.go.
var Tests = map[string]string{
	// cwm: the real corpus (modules/) copied, converted or indexed.
	"internal/extensions/cwm/internal/core:TestPlanningBehavesTheSameOnAConvertedCopy":         "converts a copy of the real corpus and replays planning on it, ~80 s",
	"internal/extensions/cwm/internal/core:TestRealCorpusIsValid":                              "validates every object of the real corpus, ~75 s",
	"internal/extensions/cwm/internal/core:TestRewritingTheCorpusChangesNoByte":                "rewrites every object file of the real corpus, ~16 s",
	"internal/extensions/cwm/internal/core:TestFullTextOnTheRealCorpus":                        "indexes the real corpus for full-text search, ~15 s",
	"internal/extensions/cwm/internal/core:TestMeasureIndexOnRealCorpus":                       "indexes the real corpus and bounds its heap per object, ~13 s",
	"internal/extensions/cwm/internal/storeset:TestTheJoinedIndexOfTwoProjectsStaysInItsBound": "copies the real corpus into two projects and bounds the joined index's heap, ~25 s",
	"internal/extensions/cwm/internal/core:TestRealCorpusEdgesValidate":                        "validates every edge of the real corpus, ~8 s",
	"internal/extensions/cwm/internal/core:TestManyChangedObjectsAreCaughtUpFast":              "writes 1,187 object files twice and opens on each, ~10 s",
	// cwm: number blocks reserved through a bare remote, one git process per step.
	"internal/extensions/cwm/internal/core:TestTwoClonesNeverMintTheSameID":                      "two clones mint turn by turn through a bare remote, a git fetch and push per block, ~30 s",
	"internal/extensions/cwm/internal/numberblock:TestTwoClonesReservingAtOnceGetDisjointBlocks": "two clones race one reservation through a bare remote, ~30 s",
	"internal/extensions/cwm/internal/numberblock:TestAProvisionalBlockIsConfirmedOrLost":        "reservations against a remote taken away and given back, ~20 s",
	// kb: extractions that probe the PDF tools through Python.
	"internal/extensions/kb/internal/core:TestTheKnowledgeBaseReadsAnItemsTextOrItsDataAsText": "two extractions, each probing the PDF and Office tools through Python, then an index and a query, ~20 s",
	// drawio: every corpus diagram laid out again.
	"internal/extensions/drawio/render:TestRelayout_Stable":                     "lays out every corpus diagram twice, ~52 s",
	"internal/extensions/drawio/render:TestRelayout_KeepsEverythingButGeometry": "lays out every corpus diagram, ~24 s",
	"internal/extensions/drawio/render:TestCorpus":                              "renders every corpus diagram, ~13 s",
	// slidedeck: a deck exported through bin/ktw-display and a headless browser.
	"internal/extensions/slidedeck/commands:TestExportedSlidesShowWhatTheDeckSays": "starts bin/ktw-display, exports a deck as pictures and as editable slides in a headless browser, ~30 s",
	// the built programs of bin/ started one by one.
	"internal/test/binaries:TestCompiledBinariesPublishSoundContributions":  "starts every program of bin/ and reads its manifest, ~60 s",
	"internal/test/binaries:TestCompiledBinariesCarryTheSystemProviderOnce": "starts every program of bin/ and reads its catalog, ~17 s",
	"internal/test/catalog:TestGUIWalk_EveryEntryOfEveryKindOpens":          "opens every catalog entry of every kind through a real universe, ~20 s",
	// planification: one running world built per test.
	"internal/extensions/planification/local:TestDeliveryRunningIncrements":                          "builds a world of three increments being built, ~15-30 s",
	"internal/extensions/planification/local:TestDeliveryRunningRequests":                            "builds a world of three increments being built, ~15-30 s",
	"internal/extensions/planification/local:TestDeliveryRunningDots":                                "builds a world of three increments being built, ~10-17 s",
	"internal/extensions/planification/local:TestPlannedDeliveryPanelShowsOpenRequests":              "builds the Running tab's world of three increments being built, ~10-17 s",
	"internal/extensions/planification/local:TestAnIncrementsPageOpensOnItsOpenRequests":             "builds the Running tab's world of three increments being built, ~10-17 s",
	"internal/extensions/planification/local:TestAnIncrementsOpenRequestCountIsItsPagesHeading":      "builds the Running tab's world of three increments being built, ~10-17 s",
	"internal/extensions/planification/local:TestTheIncrementPagesActionsSpeakTheGUIsLanguage":       "builds the Running tab's world of three increments being built, ~10-17 s",
	"internal/extensions/planification/local:TestAReleasePaneIsBuiltOfSections":                      "builds the Running tab's world of three increments being built, ~10-17 s",
	"internal/extensions/planification/local:TestAnObjectsQueueIsTheWholeQueueKeptToIt":              "builds the Running tab's world, then reads each release's and increment's queue twice, ~10-20 s",
	"internal/extensions/planification/local:TestPersonQueuePageOnEightyItemsAnswersWithinItsBudget": "files sixty asked requests and twenty decisions, then times each page read against its budget, ~10-20 s",
	// scenario: a real screen opened, stages run as processes.
	"internal/extensions/scenario/internal/machine:TestGUIStageFailsWhatDoesNotHold":           "opens a real screen and walks every failing step, ~85 s",
	"internal/extensions/scenario/internal/machine:TestGUIStageWalksTheScreenByElementIDs":     "opens a real screen and walks it, ~20 s",
	"internal/extensions/scenario/internal/machine:TestTheLocalKindRunsTheStagesInScopedRoots": "runs a case's stages as processes in roots of their own, ~15 s",
	// task: every shipped task against the provider contract.
	"internal/extensions/task/contrib:TestTaskProviders_HoldTheProviderContract": "loads every shipped task of ktw-agents through every provider, ~110 s",
	// terminal: the ktw-agents program started and spoken to.
	"internal/extensions/terminal/local:TestSmoke_TerminalSlotResponds":        "starts bin/ktw-agents and speaks to it, ~25 s",
	"internal/extensions/terminal/local:TestSmoke_RunnableSkillsAppearAsTasks": "starts bin/ktw-agents and speaks to it, ~20 s",
	"internal/extensions/terminal/local:TestSmoke_TasksProviderURI":            "starts bin/ktw-agents and speaks to it, ~20 s",
	"internal/extensions/terminal/local:TestSmoke_SkillsListResponds":          "starts bin/ktw-agents and speaks to it, ~18 s",
	"internal/extensions/terminal/local:TestSmoke_TasksListResponds":           "starts bin/ktw-agents and speaks to it, ~13 s",
}

const modulePath = "github.com/katyoti/katyoti-fwk/"

// Test skips the calling test under -short (the quick lane) when it is listed
// in Tests, and fails it when it is not: a heavy test is named in one place.
func Test(t testing.TB) {
	t.Helper()
	_, file, _, _ := runtime.Caller(1)
	key := Key(PackageDir(filepath.Dir(file)), t.Name())
	why, ok := Tests[key]
	if !ok {
		t.Fatalf("long.Test: %s has no row in internal/test/long.Tests - add it, with what the test costs", key)
	}
	if testing.Short() {
		t.Skipf("long test, skipped in the quick lane (-short): %s", why)
	}
}

// Key is a test's row key: its package folder from the module root, then its
// top-level test name.
func Key(pkgDir, testName string) string {
	if i := strings.IndexByte(testName, '/'); i >= 0 {
		testName = testName[:i]
	}
	return pkgDir + ":" + testName
}

// PackageDir is dir from the module root (the folder holding go.mod), with
// forward slashes; a -trimpath file path loses its module path.
func PackageDir(dir string) string {
	slashed := filepath.ToSlash(dir)
	if rest, ok := strings.CutPrefix(slashed, modulePath); ok {
		return rest
	}
	for root := dir; ; {
		if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
			if rel, err := filepath.Rel(root, dir); err == nil {
				return filepath.ToSlash(rel)
			}
			return slashed
		}
		parent := filepath.Dir(root)
		if parent == root {
			return slashed
		}
		root = parent
	}
}
