// SPDX-License-Identifier: Apache-2.0

// Package census provides the contribution-census guard: a binary builds the
// manifest it publishes, in-process, and that census is compared against a
// checked-in golden file.
//
// The census answers one question cheaply: DID THIS CHANGE WHAT THIS BINARY
// PUBLISHES? A refactor that is meant to move code without changing output
// leaves the golden untouched; anything else shows up as a reviewable diff
// naming the exact URIs.
//
// It runs under plain `go test` — no built binaries, no hub, no bus, no
// subprocess. `Builder.Build` assembles from the providers registered on the
// extension point, which is the same path a host binary uses to build its own
// manifest at boot.
//
// # Re-blessing an intended change
//
//	KTW_CENSUS_UPDATE=1 go test ./app/... -run TestCensus
//
// The updated golden is then reviewed like any other diff — that review IS the
// approval of the change in published output.
package census

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	manifestlocal "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/local"
	"github.com/katyoti/katyoti-fwk/internal/extensions/slidedeck/contrib/deckkind"
)

// updating reports whether goldens should be rewritten instead of compared.
//
// An environment variable rather than a test flag: a flag is only defined in
// packages that link this one, so `go test ./app/... -update` would fail in
// every package that has tests but no census. KTW_CENSUS_UPDATE works
// repo-wide.
func updating() bool { return os.Getenv("KTW_CENSUS_UPDATE") != "" }

// goldenPath is the golden's location relative to the calling package.
const goldenPath = "testdata/census.golden"

// Assert builds the manifest `exts` produce and compares its census against the
// package's golden file.
//
// The census records one line per contribution — URI, kind, content hash —
// sorted. Hashes rather than bodies keep the golden reviewable while still
// making a content change visible.
//
// Slot introspection is deliberately excluded: the builder populates it from a
// live KTSP endpoint and leaves it empty otherwise, so including it would make
// the golden depend on how the test constructs the application rather than on
// what the binary publishes.
func Assert(t *testing.T, name, version string, exts []application.Extension) {
	t.Helper()
	Isolate(t)
	AssertApp(t, name, version, application.New(application.Config(), exts...))
}

// AssertWithin is Assert for a binary that publishes entries of kinds another
// member of its universe declares (foreign kinds — a sidecar shipping a deck
// theme publishes `deck-theme` and `deck-asset`, which ktw-docs declares).
// `declared` names that universe: DeclarationsOf the member's real
// declaration folders, never a copy that could drift from them.
func AssertWithin(t *testing.T, name, version string, exts []application.Extension, declared []contribapi.Contribution) {
	t.Helper()
	Isolate(t)
	assertApp(t, name, version, application.New(application.Config(), exts...), declared)
}

// AssertApp is Assert for a binary that constructs its application itself
// rather than from a plain extension list. The application must NOT have been
// run yet — AssertApp runs it.
//
// Call Isolate first: an application reads its environment while being
// constructed, so isolation has to precede construction.
func AssertApp(t *testing.T, name, version string, app *application.Application) {
	t.Helper()
	assertApp(t, name, version, app, nil)
}

// AssertAppWithin is AssertApp for a binary that also publishes entries of
// kinds another member of its universe declares (see AssertWithin).
func AssertAppWithin(t *testing.T, name, version string, app *application.Application, declared []contribapi.Contribution) {
	t.Helper()
	assertApp(t, name, version, app, declared)
}

func assertApp(t *testing.T, name, version string, app *application.Application, declared []contribapi.Contribution) {
	t.Helper()

	closeWhenDone(t, app)

	// ONE application per process. Construction registers into package-level
	// extension points, so building twice fails on duplicate keys — which is
	// also true of the real binary, and is why the census and the schema check
	// share a single build rather than each making their own.
	m, err := buildManifest(name, version, app)
	if err != nil {
		t.Fatalf("census[%s]: build manifest: %v", name, err)
	}
	committed := committedOf(app, m)
	// Before AssertValid: its schema check constructs a second application whose
	// own publications land in the process-wide point this guard reads.
	assertEveryOfferingIsPublished(t, name, app, m)
	assertProbedManifestCarriesNoLocalEntry(t, name, app)
	assertConfigFilesHaveTemplates(t, name, m)
	AssertValidWithin(t, name, m, SchemaService(t), declared)
	assertContentLintClean(t, name, m, sourcesOf(app))
	lines := censusLines(m, committed)
	got := strings.Join(lines, "\n") + "\n"

	if updating() {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
			t.Fatalf("census[%s]: mkdir testdata: %v", name, err)
		}
		if err := os.WriteFile(goldenPath, []byte(got), 0o644); err != nil {
			t.Fatalf("census[%s]: write golden: %v", name, err)
		}
		t.Logf("census[%s]: golden updated (%d contributions)", name, len(lines))
		return
	}

	wantBytes, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("census[%s]: read golden (%s): %v\n"+
			"If this binary is new, create it with:  KTW_CENSUS_UPDATE=1 go test ./app/%s/ -run TestCensus",
			name, goldenPath, err, name)
	}
	// Normalise line endings: the golden is generated with LF but may be
	// checked out with CRLF wherever git is configured to translate, which
	// would otherwise fail every line on a fresh clone.
	want := normalise(string(wantBytes))
	if normalise(got) == want {
		return
	}

	t.Errorf("census[%s]: published contributions changed.\n%s\n"+
		"If this change is intended, re-bless and review the diff:\n"+
		"  KTW_CENSUS_UPDATE=1 go test ./app/%s/ -run TestCensus",
		name, diff(want, normalise(got)), name)
}

// Isolate points the process at an empty workspace, data and cache directory
// for the duration of the test, so nothing the application does at boot touches
// the developer's real universe and nothing in that universe leaks into the
// census.
//
// Must be called BEFORE the application is constructed.
func Isolate(t *testing.T) {
	t.Helper()
	t.Setenv("KTW_WORKSPACE", t.TempDir())
	t.Setenv("KTW_DATA", t.TempDir())
	t.Setenv("KTW_CACHE", t.TempDir())
}

// closeWhenDone closes app when the test ends, as a command's application is
// closed when it returns: its services give up what they hold in the isolated
// folders — the task run store's owner lock among them, a file Windows will
// not remove while it is open. Registered after Isolate's TempDirs, it runs
// before they are removed (cleanups run last-in, first-out).
func closeWhenDone(t *testing.T, app *application.Application) {
	t.Cleanup(func() {
		appcore.CloseApplicationScope(application.WithApplication(context.Background(), app))
	})
}

// buildManifest runs the application and returns the manifest it publishes.
// Shared by the census guard and the source-side validation check.
func buildManifest(name, version string, app *application.Application) (*mbapi.Manifest, error) {
	if err := app.Run(); err != nil {
		return nil, fmt.Errorf("application run: %w", err)
	}
	ctx := application.WithApplication(context.Background(), app)
	m, err := manifestlocal.NewBuilder(name, version).Build(ctx)
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, fmt.Errorf("builder returned no manifest")
	}
	return m, nil
}

// censusLines renders a manifest as census lines, each hash read through
// committed: what a clean checkout of the same commit would hash (committed.go).
func censusLines(m *mbapi.Manifest, committed committedHashes) []string {
	lines := make([]string, 0, len(m.Contributions))
	for _, c := range m.Contributions {
		hash := committed.layerHash(c.DataHash())
		if hash == "" {
			hash = "-"
		}
		// Layers are part of what a binary publishes, so they belong in the
		// census: without them a layer appearing, vanishing, or changing its
		// content would pass unnoticed.
		//
		// `data` is deliberately omitted: it IS the hash column above. Printing
		// it twice would make every line change the day the body became a
		// layer, and a guard whose every line moves at once tells a reviewer
		// nothing.
		var layers []string
		for name, l := range c.Layers {
			if name == contribapi.LayerData {
				continue
			}
			layers = append(layers, fmt.Sprintf("%s=%s", name, committed.layerHash(l.Hash)))
		}
		sort.Strings(layers)
		lines = append(lines, fmt.Sprintf("%s\t%s\t%s\t%s",
			c.ResolvedURI(), c.Type, hash, strings.Join(layers, ",")))
	}
	// Every producer's identity, as one line each. A provider is part of what
	// a binary publishes as much as its entries are: whether a source can
	// change decides whether anything downstream may cache it (G4).
	for _, p := range m.Providers {
		mut := "immutable"
		if p.Mutable {
			mut = "mutable"
		}
		// The digest column is the provider's epoch: an immutable source's
		// enumeration digest, "-" for a mutable one (it has no single
		// version). An immutable row with "-" breaks the identity rule and is
		// refused by assertImmutableProvidersCarryADigest.
		digest := committed.providerDigest(p.Digest)
		if digest == "" {
			digest = "-"
		}
		lines = append(lines, fmt.Sprintf("provider:%s/%s\tidentity\t%s\t%s", p.Name, p.Type, mut, digest))
	}
	sort.Strings(lines)
	return lines
}

// diff renders the added / removed / changed lines between two censuses, so a
// failure names what moved instead of dumping both files.
func diff(want, got string) string {
	wantSet := lineSet(want)
	gotSet := lineSet(got)

	// Key by URI so a changed hash reads as "changed", not as one add + one
	// removal.
	wantByURI := byURI(wantSet)
	gotByURI := byURI(gotSet)

	var added, removed, changed []string
	for uri, g := range gotByURI {
		w, ok := wantByURI[uri]
		switch {
		case !ok:
			added = append(added, "  + "+uri)
		case w != g:
			changed = append(changed, "  ~ "+uri)
		}
	}
	for uri := range wantByURI {
		if _, ok := gotByURI[uri]; !ok {
			removed = append(removed, "  - "+uri)
		}
	}
	sort.Strings(added)
	sort.Strings(removed)
	sort.Strings(changed)

	var b strings.Builder
	report := func(label string, items []string) {
		if len(items) == 0 {
			return
		}
		fmt.Fprintf(&b, "%s (%d):\n%s\n", label, len(items), strings.Join(items, "\n"))
	}
	report("removed", removed)
	report("added", added)
	report("content changed", changed)
	if b.Len() == 0 {
		return "  (line ordering differs but the set is identical)"
	}
	return b.String()
}

// normalise strips carriage returns so a CRLF checkout compares equal to the
// LF the census generates.
func normalise(s string) string { return strings.ReplaceAll(s, "\r\n", "\n") }

func lineSet(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return out
}

func byURI(lines []string) map[string]string {
	out := make(map[string]string, len(lines))
	for _, l := range lines {
		uri, _, _ := strings.Cut(l, "\t")
		out[uri] = l
	}
	return out
}

// sourceKinds are the kinds whose source the content lint holds to a format
// check with a required source layer: their bytes are read for it.
var sourceKinds = map[string]bool{
	mbapi.ResourceTypeSkill: true, mbapi.ResourceTypeTask: true,
	deckkind.KindDeckTheme: true, deckkind.KindSlideGroup: true,
}

// sourcesOf reads every skill, task, deck theme and slide group SOURCE this
// binary's providers carry —
// from the providers' own enumeration, where the bytes still are (the built
// manifest has residency applied and drops them). Test code only: nothing
// here links into the executable.
func sourcesOf(app *application.Application) map[string][]byte {
	ctx := application.WithApplication(context.Background(), app)
	out := map[string][]byte{}
	for _, p := range application.UseAppExtensions[contribapi.ContributionProvider](app) {
		if !sourceKinds[p.Type()] {
			continue
		}
		items, err := p.Enumerate(ctx)
		if err != nil {
			continue
		}
		for _, c := range items {
			if src := c.Source(); len(src) > 0 {
				out[c.ResolvedURI()] = src
			}
		}
	}
	return out
}
