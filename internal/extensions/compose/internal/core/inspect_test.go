// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/appcore/hubclient"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	api "github.com/katyoti/katyoti-fwk/internal/extensions/compose/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	manifestremote "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/remote"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp/messagebus"
	"github.com/katyoti/katyoti-fwk/internal/test/hubharness"
)

// ── in-process: a kind the test invents, answered by the registry ─────────────

// TestInspect_AnyKindThroughTheRegistry: a kind no code knows, declared by a
// contribution-type entry in the manifest, is inspected completely — schema,
// declared layers, default, budget, severity, execution, identity, bands — and
// a layer the entry carries but the kind never declared shows as such.
func TestInspect_AnyKindThroughTheRegistry(t *testing.T) {
	decl := contribapi.Contribution{Provider: "ktw-w", Type: contribapi.ResourceTypeContributionType, ID: "widget"}
	decl.SetData([]byte(`{"name":"widget","schema":"ktw-w:/data-type/widget","validate":"error","execution":"executable",
		"layers":{"data":{"mediaType":"application/json","required":true},"source":{"mediaType":"text/plain","default":true}},
		"meta":{"budget":64}}`))
	entry := contribapi.Contribution{Provider: "ktw-w", Type: "widget", ID: "w1",
		Metadata: &contribapi.ContributionMeta{Name: "W one", Category: "tools"}}
	entry.SetData([]byte(`{"n":1}`))
	entry.SetSource([]byte(strings.Repeat("x", contribapi.BandInline+1)), "text/plain")
	entry.SetLayer("extra", []byte("e"), "text/plain")
	entry.DropInline() // as an index carries it: inline band keeps its bytes, the rest a descriptor
	ok := false
	entry.Valid, entry.ValidationErrors = &ok, []string{"n: want string"}
	entry.Provenance = &contribapi.Provenance{Signature: "sig", KeyID: "k1"}
	s := &Service{exeOverride: fakeExes{manifests: map[string]*mbapi.Manifest{"bin/ktw-w.exe": {
		Provider:      "ktw-w",
		Contributions: []contribapi.Contribution{decl, entry},
		Providers:     []mbapi.ProviderInfo{{Name: "ktw-w", Type: "widget", Digest: "sha256:d", Mutable: true}},
	}}}}

	out, err := s.inspect(context.Background(), api.InspectInput{URI: "ktw-w:/widget/w1", Exe: "bin/ktw-w.exe"})
	if err != nil {
		t.Fatal(err)
	}
	k := out.Kind
	if !k.Declared || k.DeclaredBy != "ktw-w:/contribution-type/widget" || k.Schema != "ktw-w:/data-type/widget" ||
		k.DefaultLayer != "source" || k.Budget != 64 || k.Validate != "on" || k.Execution != "executable" || len(k.Layers) != 2 {
		t.Errorf("kind = %+v", k)
	}
	byName := map[string]api.InspectLayer{}
	for _, l := range out.Layers {
		byName[l.Name] = l
	}
	if out.Layers[0].Name != "source" {
		t.Errorf("default layer not first: %+v", out.Layers)
	}
	if l := byName["source"]; !l.Declared || !l.Carried || l.Band != string(contribapi.BandNameControl) || l.Inline {
		t.Errorf("source layer = %+v, want declared, carried, control band, not inline", l)
	}
	if l := byName["data"]; l.Band != string(contribapi.BandNameInline) || !l.Inline || l.Hash != entry.DataHash() {
		t.Errorf("data layer = %+v", l)
	}
	if l := byName["extra"]; l.Declared || !l.Carried {
		t.Errorf("undeclared carried layer = %+v, want carried and not declared", l)
	}
	if len(out.Provider.Identity) != 1 || out.Provider.Identity[0].Digest != "sha256:d" || !out.Provider.Identity[0].Mutable {
		t.Errorf("provider = %+v", out.Provider)
	}
	if out.Verdict.State != "invalid" || len(out.Verdict.Errors) != 1 || out.Verdict.Note != "" {
		t.Errorf("verdict = %+v", out.Verdict)
	}
	if out.Provenance == nil || !out.Provenance.Signed || out.Provenance.IdentityDigest != entry.IdentityDigest() {
		t.Errorf("provenance = %+v", out.Provenance)
	}
	if out.Meta["name"] != "W one" || out.Meta["category"] != "tools" {
		t.Errorf("meta = %v", out.Meta)
	}

	// An entry the source does not publish, and every malformed question, is an
	// error — never an empty document.
	if _, err := s.inspect(context.Background(), api.InspectInput{URI: "ktw-w:/widget/none", Exe: "bin/ktw-w.exe"}); err == nil {
		t.Error("an unpublished URI must be an error")
	}
	for _, bad := range []api.InspectInput{
		{URI: "ktw-w:/widget/w1"},
		{URI: "ktw-w:/widget/w1", Exe: "x", Universe: true},
		{Exe: "x"},
		{URI: "not a uri", Exe: "x"},
	} {
		if _, err := s.inspect(context.Background(), bad); err == nil {
			t.Errorf("%+v: want an error", bad)
		}
	}
}

// ── across processes: the e19 fixture pack served by bin/ktw-host ────────────

// pack is the files-only fixture as its FILES say it: the provider token, the
// one kind it declares (name, schema, layers) and the one entry of it. Read
// from disk so this test names no kind of its own.
type pack struct {
	root, provider string
	kind           contribapi.KindDeclaration
	entryURI       string
	entrySource    []byte
}

func readPack(t *testing.T) pack {
	t.Helper()
	root := filepath.Join(repoRootDir(t), "internal", "test", "fixtures", "contributions", "files-only-pack")
	var host struct {
		Name string            `yaml:"name"`
		Dirs map[string]string `yaml:"dirs"`
	}
	decode(t, filepath.Join(root, "ktw-host.yaml"), &host)
	kinds, err := filepath.Glob(filepath.Join(root, "contribution-types", "*.yaml"))
	if err != nil || len(kinds) != 1 {
		t.Fatalf("fixture declares %d kinds (%v), want 1", len(kinds), err)
	}
	var raw map[string]any
	decode(t, kinds[0], &raw)
	b, _ := json.Marshal(raw)
	var decl contribapi.KindDeclaration
	if err := json.Unmarshal(b, &decl); err != nil {
		t.Fatal(err)
	}
	entries, _ := filepath.Glob(filepath.Join(root, filepath.FromSlash(host.Dirs[decl.Name]), "*.yaml"))
	if len(entries) != 1 {
		t.Fatalf("fixture has %d entries of %s, want 1", len(entries), decl.Name)
	}
	src, err := os.ReadFile(entries[0])
	if err != nil {
		t.Fatal(err)
	}
	id := strings.TrimSuffix(filepath.Base(entries[0]), filepath.Ext(entries[0]))
	return pack{root: root, provider: host.Name, kind: decl,
		entryURI: fmt.Sprintf("%s:/%s/%s", host.Name, decl.Name, id), entrySource: src}
}

func decode(t *testing.T, path string, into any) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := marshal.Unmarshal(b, marshal.YAML, into); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
}

func repoRootDir(t *testing.T) string {
	t.Helper()
	_, here, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(here), "..", "..", "..", "..", "..")
}

func binary(t *testing.T, name string) string {
	t.Helper()
	return hubharness.Binary(t, name)
}

// assertPackDocument holds the inspect document to what the fixture files say.
func assertPackDocument(t *testing.T, p pack, out api.InspectOutput) {
	t.Helper()
	if out.URI != p.entryURI || out.Kind.Name != p.kind.Name || !out.Kind.Declared {
		t.Errorf("document names %s of kind %q (declared %v), want %s", out.URI, out.Kind.Name, out.Kind.Declared, p.entryURI)
	}
	if out.Kind.Schema != p.kind.Schema {
		t.Errorf("kind schema = %q, want the pack's %q", out.Kind.Schema, p.kind.Schema)
	}
	if want := fmt.Sprintf("%s:/%s/%s", p.provider, contribapi.ResourceTypeContributionType, p.kind.Name); out.Kind.DeclaredBy != want {
		t.Errorf("kind declared by %q, want %q", out.Kind.DeclaredBy, want)
	}
	if out.Kind.DefaultLayer != p.kind.DefaultLayer() || out.Kind.Budget != p.kind.Budget() || out.Kind.Validate != p.kind.Severity() {
		t.Errorf("kind = %+v, want default %q budget %d severity %q", out.Kind, p.kind.DefaultLayer(), p.kind.Budget(), p.kind.Severity())
	}
	if len(out.Kind.Layers) != len(p.kind.Layers) {
		t.Errorf("kind declares %d layers, document shows %d", len(p.kind.Layers), len(out.Kind.Layers))
	}
	got := map[string]api.InspectLayer{}
	for _, l := range out.Layers {
		got[l.Name] = l
	}
	for name, ld := range p.kind.Layers {
		l, ok := got[name]
		switch {
		case !ok || !l.Declared || !l.Carried:
			t.Errorf("layer %s: %+v, want declared and carried", name, l)
		case l.Band != string(contribapi.BandOf(l.Size)) || l.Hash == "":
			t.Errorf("layer %s: band %q hash %q for size %d", name, l.Band, l.Hash, l.Size)
		case l.MediaType != ld.MediaType:
			t.Errorf("layer %s: media type %q, want %q", name, l.MediaType, ld.MediaType)
		}
	}
	// The source layer IS the file as written: its hash is the file's.
	if l := got[contribapi.LayerSource]; l.Hash != contribapi.HashContent(p.entrySource) {
		t.Errorf("source layer hash %s, want the fixture file's %s", l.Hash, contribapi.HashContent(p.entrySource))
	}
	if out.Provider.Name != p.provider || len(out.Provider.Identity) == 0 {
		t.Fatalf("provider = %+v, want %s with an identity row", out.Provider, p.provider)
	}
	for _, r := range out.Provider.Identity {
		if r.Type != p.kind.Name || !r.Mutable || r.Digest != "" {
			t.Errorf("identity row %+v, want mutable with no digest (a folder can change) for kind %s", r, p.kind.Name)
		}
	}
}

// TestInspect_FilesOnlyPackFromTheHost runs the verb's --exe path against the
// generic host serving the e19 pack: the kind the pack declares, its schema,
// both layers with their bands, the provider's identity — and the verdict,
// which a single binary does not carry and the document says so.
func TestInspect_FilesOnlyPackFromTheHost(t *testing.T) {
	host := binary(t, "ktw-host")
	if host == "" {
		hubharness.MissingBinary(t, "inspect --exe: bin/ktw-host not built")
		return
	}
	p := readPack(t)
	t.Setenv("KTW_CONTENT_ROOT", p.root) // the host's folder, inherited by the process
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	out, err := NewInspector().Inspect(ctx, api.InspectInput{URI: p.entryURI, Exe: host})
	if err != nil {
		t.Fatal(err)
	}
	assertPackDocument(t, p, out)
	if out.Source != "exe:"+host {
		t.Errorf("source = %q", out.Source)
	}
	if out.Verdict.State != "unchecked" || out.Verdict.Valid != nil || out.Verdict.Note == "" {
		t.Errorf("verdict = %+v, want unchecked with the note naming --universe", out.Verdict)
	}
}

// TestInspect_FilesOnlyPackInTheUniverse runs the --universe path: the pack as a
// member of a real hub, read through the aggregate. Here the verdict exists —
// the hub validated the entry against the pack's own schema.
func TestInspect_FilesOnlyPackInTheUniverse(t *testing.T) {
	host := binary(t, "ktw-host")
	if host == "" {
		hubharness.MissingBinary(t, "inspect --universe: bin/ktw-host not built")
		return
	}
	p := readPack(t)
	folder := filepath.Join(t.TempDir(), "pack")
	if err := os.CopyFS(folder, os.DirFS(p.root)); err != nil {
		t.Fatal(err)
	}
	const universe = "inspect-pack"
	ws := t.TempDir()
	cfg := filepath.Join(ws, ".katyoti", "config")
	if err := os.MkdirAll(cfg, 0o755); err != nil {
		t.Fatal(err)
	}
	f, h := filepath.ToSlash(folder), filepath.ToSlash(host)
	univers := fmt.Sprintf("version: 1\nname: %s\ndisplay_name: %s\nicon: T\ncolor: gray\nsidecars:\n"+
		"  - name: %s\n    transport: stdio\n    binary: %s\n    content_root: %s\n    args: [\"--folder\", \"%s\", \"sc\", \"ktsp\", \"stdio\"]\n",
		universe, universe, p.provider, h, f, f)
	if err := os.WriteFile(filepath.Join(cfg, "Univers.yaml"), []byte(univers), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfg, "Settings.yaml"), []byte("# inspect test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KTW_WORKSPACE", ws)
	dataDir := filepath.Join(ws, ".katyoti")
	stop := hubharness.Start(t, dataDir)
	t.Cleanup(stop)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	hc, err := hubclient.ConnectToHub(ctx, hubclient.ConnectOptions{DataDir: dataDir, Mode: hubclient.ModeReuse})
	if err != nil {
		t.Fatalf("connect to hub: %v", err)
	}
	t.Cleanup(func() { hc.Close() })
	s := &Service{universeOverride: manifestremote.New(messagebus.NewSender(hc.MessageBus(), "local", universe))}

	// The hub hydrates its members after it is reachable: wait for the entry
	// and its verdict rather than for a fixed time.
	var out api.InspectOutput
	for {
		out, err = s.inspect(ctx, api.InspectInput{URI: p.entryURI, Universe: true})
		if err == nil && out.Verdict.State != "unchecked" {
			break
		}
		if ctx.Err() != nil {
			t.Fatalf("universe never served %s with a verdict: last err %v, verdict %+v", p.entryURI, err, out.Verdict)
		}
		time.Sleep(500 * time.Millisecond)
	}
	assertPackDocument(t, p, out)
	if out.Source != "universe" {
		t.Errorf("source = %q", out.Source)
	}
	if out.Verdict.State != "valid" || out.Verdict.Valid == nil || !*out.Verdict.Valid || len(out.Verdict.Errors) > 0 {
		t.Errorf("verdict = %+v, want valid against %s", out.Verdict, p.kind.Schema)
	}
}
