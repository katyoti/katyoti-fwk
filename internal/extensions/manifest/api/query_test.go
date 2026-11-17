// SPDX-License-Identifier: Apache-2.0

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
)

// queryCatalog is a Catalog holding one manifest, enough to exercise the
// universe reach without the hub.
type queryCatalog struct {
	Catalog
	entries []contribapi.Contribution
}

func (c *queryCatalog) GetContributionsByType(_ context.Context, t string) ([]contribapi.Contribution, error) {
	var out []contribapi.Contribution
	for _, e := range c.entries {
		if e.Type == t {
			out = append(out, e)
		}
	}
	return out, nil
}

func (c *queryCatalog) GetBody(_ context.Context, uri string) ([]byte, error) {
	for _, e := range c.entries {
		if e.ResolvedURI() == uri {
			return e.Data(), nil
		}
	}
	return nil, errors.New("no body")
}

// OpenLayer: the aggregate serves the NAMED layer from its own index. This is
// what "reads through the aggregate" means for the content lane — the old
// fallback to GetBody served the data layer whatever was asked.
func (c *queryCatalog) OpenLayer(_ context.Context, uri, layer string) (io.ReadCloser, contribapi.LayerRef, error) {
	for _, e := range c.entries {
		if e.ResolvedURI() == uri {
			ref, ok := e.Layers[layer]
			if !ok {
				return nil, contribapi.LayerRef{}, errors.New("no such layer")
			}
			inline := ref.Inline
			ref.Inline = nil
			return io.NopCloser(bytes.NewReader(inline)), ref, nil
		}
	}
	return nil, contribapi.LayerRef{}, errors.New("no such entry")
}

func fixture() *queryCatalog {
	body := func(name string) json.RawMessage {
		return json.RawMessage(`{"name":"` + name + `"}`)
	}
	return &queryCatalog{entries: []contribapi.Contribution{
		{Provider: "ktw-docs", Type: "skill", ID: "doc-create", Layers: contribapi.InlineData(body("doc-create"))},
		{Provider: contribapi.SystemProvider, Type: "skill", ID: "sys-skill", Layers: contribapi.InlineData(body("sys-skill"))},
		{Provider: contribapi.LocalProvider, Type: "skill", ID: "debug", Layers: contribapi.InlineData(body("debug"))},
		{Provider: "ktw-docs", Type: "task", ID: "render", Layers: contribapi.InlineData(body("render"))},
	}}
}

func ids(t *testing.T, cs []contribapi.Contribution) []string {
	t.Helper()
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		out = append(out, c.ID)
	}
	return out
}

func eq(t *testing.T, label string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s = %v, want %v", label, got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("%s = %v, want %v", label, got, want)
		}
	}
}

// TestQuery_reachIsMandatory asserts a Q with no reach is refused rather than
// defaulting. Defaulting is the bug this surface replaces: a consumer that
// meant the universe and silently got only its own contributions had no way
// to find out.
func TestQuery_reachIsMandatory(t *testing.T) {
	_, err := Query(context.Background(), Q{Kind: "skill", Catalog: fixture()})
	if !errors.Is(err, ErrReachUnset) {
		t.Fatalf("Query with no reach: err = %v, want ErrReachUnset", err)
	}
	if _, err := Open(context.Background(), Q{}, "ktw-docs:/skill/doc-create", ""); !errors.Is(err, ErrReachUnset) {
		t.Fatalf("Open with no reach: err = %v, want ErrReachUnset", err)
	}
}

// TestQuery_visibilityDefaultsPerReach asserts the two visibility defaults:
// system is included at either reach, and local is included at self but not at
// universe — a local contribution does not leave its binary, so asking the
// universe for one is asking for something that cannot be there.
func TestQuery_visibilityDefaultsPerReach(t *testing.T) {
	cat := fixture()
	got, err := Query(context.Background(), Q{Reach: ReachUniverse, Kind: "skill", Catalog: cat})
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "universe default", ids(t, got), []string{"doc-create", "sys-skill"})

	// Asking for it explicitly overrides the default in both directions.
	got, err = Query(context.Background(), Q{Reach: ReachUniverse, Kind: "skill", Catalog: cat,
		Filter: Filter{IncludeLocal: IncludeYes, IncludeSystem: IncludeNo}})
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "universe local-yes system-no", ids(t, got), []string{"doc-create", "debug"})
}

// TestQuery_providerFilter asserts the provider axis narrows independently of
// the system/local axes.
func TestQuery_providerFilter(t *testing.T) {
	got, err := Query(context.Background(), Q{Reach: ReachUniverse, Kind: "skill", Catalog: fixture(),
		Filter: Filter{Providers: []string{"ktw-docs"}}})
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "providers=ktw-docs", ids(t, got), []string{"doc-create"})
}

// TestQueryTyped_decodesAndAttributes asserts bodies decode and carry their
// own provider, not the manifest's — a manifest may hold contributions for
// other providers and attributing them to the carrier breaks dedupe.
func TestQueryTyped_decodesAndAttributes(t *testing.T) {
	type skill struct {
		Name string `json:"name"`
	}
	got, err := QueryTyped[skill](context.Background(),
		Q{Reach: ReachUniverse, Kind: "task", Catalog: fixture()})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Provider != "ktw-docs" || got[0].Body.Name != "render" {
		t.Fatalf("QueryTyped = %+v", got)
	}
}

// TestOpen_bandsDecideWhatMayBeHeld asserts the size band, not a declaration,
// decides how a layer travels — and that Bytes refuses the bulk band BEFORE
// transferring anything.
func TestOpen_bandsDecideWhatMayBeHeld(t *testing.T) {
	for _, tc := range []struct {
		size int64
		want contribapi.Band
	}{
		{1, contribapi.BandNameInline},
		{contribapi.BandInline, contribapi.BandNameInline},
		{contribapi.BandInline + 1, contribapi.BandNameControl},
		{contribapi.BandControl, contribapi.BandNameControl},
		{contribapi.BandControl + 1, contribapi.BandNameBulk},
		{40 << 30, contribapi.BandNameBulk},
	} {
		if got := contribapi.BandOf(tc.size); got != tc.want {
			t.Errorf("BandOf(%d) = %q, want %q", tc.size, got, tc.want)
		}
	}

	bulk := &Layer{URI: "ktw-docs:/reference/dataset.bin", Name: contribapi.LayerSource,
		Ref: contribapi.LayerRef{Hash: "sha256:x", Size: 40 << 30}}
	if _, err := bulk.Bytes(); !errors.Is(err, ErrLayerTooLargeForMemory) {
		t.Errorf("Bytes on a bulk layer: err = %v, want ErrLayerTooLargeForMemory", err)
	}
}

// TestLayerRef_carriesNoLocator asserts the descriptor stored in the index
// holds identity and size but no ADDRESS. A locator can expire or be scoped to
// one environment; storing one would make the index expire with it.
func TestLayerRef_carriesNoLocator(t *testing.T) {
	b, err := json.Marshal(contribapi.LayerRef{Hash: "sha256:abc", Size: 99, MediaType: "text/markdown"})
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"url", "uri", "locator", "href", "endpoint", "://"} {
		if bytes.Contains(bytes.ToLower(b), []byte(forbidden)) {
			t.Errorf("LayerRef serialises %q — locators must be minted at fetch time, never stored: %s", forbidden, b)
		}
	}
}

// selfService is a ResourceService holding one binary's own view.
type selfService struct {
	ResourceService
	entries []contribapi.Contribution
	listed  int
}

func (s *selfService) List(_ context.Context, in ContributionListInput) (ContributionListOutput, error) {
	s.listed++
	// The self reach must ask wide open and let Filter.Apply narrow, so the
	// visibility defaults live in one place rather than two.
	if !in.IncludeLocal || in.IncludeSystem == nil || !*in.IncludeSystem {
		return ContributionListOutput{}, errors.New("self reach narrowed visibility before Filter.Apply")
	}
	var out []contribapi.Contribution
	for _, e := range s.entries {
		if len(in.Filter.Types) == 0 || in.Filter.Types[0] == e.Type {
			out = append(out, e)
		}
	}
	return ContributionListOutput{Contributions: out}, nil
}

// withSelfService wires a ResourceService — and deliberately NO Catalog —
// into a fresh Application context.
func withSelfService(t *testing.T, svc ResourceService) context.Context {
	t.Helper()
	app := application.New(application.Config())
	application.PutService[ResourceService](app, "default", svc)
	return application.WithApplication(context.Background(), app)
}

// TestQuery_selfNeedsNoAggregate asserts the self reach answers from the
// binary's own service, with no catalog supplied and none resolvable — the
// property a standalone sidecar depends on.
func TestQuery_selfNeedsNoAggregate(t *testing.T) {
	svc := &selfService{entries: fixture().entries}
	ctx := withSelfService(t, svc)

	got, err := Query(ctx, Q{Reach: ReachSelf, Kind: "skill"})
	if err != nil {
		t.Fatal(err)
	}
	// Local IS included at self reach: it is the one place a binary-private
	// contribution is visible.
	eq(t, "self default", ids(t, got), []string{"doc-create", "sys-skill", "debug"})
	if svc.listed != 1 {
		t.Errorf("ResourceService.List called %d times, want 1", svc.listed)
	}
}

// TestOpen_universeReadsThroughAggregate asserts Open at universe reach goes
// through the aggregation point rather than the binary's own providers, and
// returns a handle that has read nothing yet.
func TestOpen_universeReadsThroughAggregate(t *testing.T) {
	l, err := Open(context.Background(), Q{Reach: ReachUniverse, Catalog: fixture()},
		"ktw-docs:/skill/doc-create", contribapi.LayerData)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	if l.Band() != contribapi.BandNameInline {
		t.Errorf("band = %q, want inline", l.Band())
	}
	got, err := l.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"name":"doc-create"}` {
		t.Fatalf("Open = %q", got)
	}
}

// TestLayer_WriteFileStreamsAndVerifies asserts content reaches disk verified
// against its own address, and that content failing its address leaves no file
// — a consumer would use a bad file, so leaving one is worse than leaving none.
func TestLayer_WriteFileStreamsAndVerifies(t *testing.T) {
	content := []byte("# a reference document")
	good := &Layer{URI: "u", Name: "source",
		Ref: contribapi.LayerRef{Hash: contribapi.HashContent(content), Size: int64(len(content))},
		rc:  newBytesReadCloser(content)}
	path := filepath.Join(t.TempDir(), "out.md")
	n, err := good.WriteFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if n != int64(len(content)) {
		t.Errorf("wrote %d bytes, want %d", n, len(content))
	}
	if b, _ := os.ReadFile(path); string(b) != string(content) {
		t.Errorf("file = %q", b)
	}

	bad := &Layer{URI: "u", Name: "source",
		Ref: contribapi.LayerRef{Hash: "sha256:" + strings.Repeat("0", 64), Size: int64(len(content))},
		rc:  newBytesReadCloser(content)}
	badPath := filepath.Join(t.TempDir(), "bad.md")
	if _, err := bad.WriteFile(badPath); err == nil {
		t.Error("content that fails its own address was accepted")
	}
	if _, err := os.Stat(badPath); !os.IsNotExist(err) {
		t.Error("a failed verification left a file behind")
	}
}

// TestKindOf_ReportsExecutionClass asserts a consumer can ask how a kind's
// content is consumed BEFORE consuming it — and that the answer comes from the
// kind, so a producer cannot promote its own entry to executable.
func TestKindOf_ReportsExecutionClass(t *testing.T) {
	decl := func(id, execution string) contribapi.Contribution {
		body := `{"name":"` + id + `","schema":"system:/data-type/` + id + `"`
		if execution != "" {
			body += `,"execution":"` + execution + `"`
		}
		body += `}`
		return contribapi.Contribution{
			Provider: contribapi.SystemProvider,
			Type:     contribapi.ResourceTypeContributionType,
			ID:       id, Layers: contribapi.InlineData(json.RawMessage(body)),
		}
	}
	cat := &queryCatalog{entries: []contribapi.Contribution{
		decl("task", "executable"),
		decl("skill", ""),
	}}
	q := Q{Reach: ReachUniverse, Catalog: cat}

	for kind, want := range map[string]contribapi.Execution{
		"task":  contribapi.ExecutionExecutable,
		"skill": contribapi.ExecutionDeclarative,
		// A kind nothing declares is declarative: the safe assumption about
		// an unknown kind is that nothing will run it.
		"who-knows": contribapi.ExecutionDeclarative,
	} {
		kinds, err := KindsAt(context.Background(), q)
		if err != nil {
			t.Fatalf("KindsAt: %v", err)
		}
		if got := kinds.Execution(kind); got != want {
			t.Errorf("Kinds.Execution(%q) = %q, want %q", kind, got, want)
		}
	}
}

// An aggregate that cannot open layers is an ERROR at universe reach, not a
// downgrade to the body lane: a caller asked for a named layer.
func TestOpen_universeRefusesAnAggregateThatCannotOpenLayers(t *testing.T) {
	type bodyOnly struct{ CatalogReader }
	_, err := Open(context.Background(), Q{Reach: ReachUniverse, Catalog: bodyOnly{fixture()}},
		"ktw-docs:/skill/doc-create", contribapi.LayerData)
	if err == nil || !strings.Contains(err.Error(), "cannot open layers") {
		t.Fatalf("want a refusal naming the aggregate, got %v", err)
	}
}
