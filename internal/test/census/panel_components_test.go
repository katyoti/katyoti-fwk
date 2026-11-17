// SPDX-License-Identifier: Apache-2.0

package census

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/contribution"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	manifestcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/contrib"
	objectpaneapi "github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/api"
	schemaapi "github.com/katyoti/katyoti-fwk/internal/extensions/schema/api"
)

// shippedPackEntries is every entry of the shipped CWM packs (each binary's
// embed/cwm and each extension's contrib/embed/cwm), read through the pack
// producer against the system kinds, with the data-types they ship.
func shippedPackEntries(t *testing.T) []contribapi.Contribution {
	t.Helper()
	sys, err := contribution.SystemKindEntries()
	if err != nil {
		t.Fatal(err)
	}
	kinds := contribapi.KindsFrom(sys)
	root := repoRoot(t)
	var packs []string
	for _, pattern := range []string{"app/*/embed/cwm", "internal/extensions/*/contrib/embed/cwm"} {
		found, _ := filepath.Glob(filepath.Join(root, filepath.FromSlash(pattern)))
		packs = append(packs, found...)
	}
	sort.Strings(packs)
	var out []contribapi.Contribution
	for _, p := range packs {
		dir, _ := filepath.Rel(root, p)
		// A binary's pack publishes under the binary's name; an extension's
		// pack under a name of its folder (its host is chosen at link time).
		provider := strings.ReplaceAll(filepath.ToSlash(dir), "/", "-")
		if parts := strings.Split(filepath.ToSlash(dir), "/"); parts[0] == "app" {
			provider = parts[1]
		}
		for _, pr := range manifestcontrib.PackProviders(provider, loader.NewFSLoader(p), ".", kinds) {
			cs, err := pr.Enumerate(context.Background())
			if err != nil {
				t.Fatalf("census: read %s of %s: %v", pr.Type(), dir, err)
			}
			for i := range cs {
				if cs[i].URI == "" {
					cs[i].URI = cs[i].ResolvedURI()
				}
			}
			out = append(out, cs...)
		}
	}
	return out
}

// shippedPanelFiles counts the object-panel files of the shipped packs (an
// .i18n.yaml beside one is its translation, not a panel).
func shippedPanelFiles(t *testing.T) int {
	t.Helper()
	root := repoRoot(t)
	n := 0
	for _, pattern := range []string{"app/*/embed/cwm/object-panels", "internal/extensions/*/contrib/embed/cwm/object-panels"} {
		dirs, _ := filepath.Glob(filepath.Join(root, filepath.FromSlash(pattern)))
		for _, d := range dirs {
			err := filepath.WalkDir(d, func(path string, e os.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if !e.IsDir() && strings.HasSuffix(path, ".yaml") && !strings.HasSuffix(path, ".i18n.yaml") {
					n++
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	return n
}

func entriesOfKind(cs []contribapi.Contribution, kind string) []contribapi.Contribution {
	var out []contribapi.Contribution
	for _, c := range cs {
		if c.Type == kind {
			out = append(out, c)
		}
	}
	return out
}

// fixtureEntry is an entry of kind with body as its data layer.
func fixtureEntry(t *testing.T, kind, id string, body any) contribapi.Contribution {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	return contribapi.Contribution{Provider: "ktw-example", Type: kind, ID: id,
		Layers: map[string]contribapi.LayerRef{contribapi.LayerData: {Inline: b}}}
}

// TestAnObjectPanelNamingAnUnknownComponentIsRefused: a panel naming a view no
// panel-component entry publishes is a finding naming the panel and the view;
// a static panel whose data does not match its view's input data-type is one
// too; a panel naming a published view with matching data, and a panel with a
// source, pass.
func TestAnObjectPanelNamingAnUnknownComponentIsRefused(t *testing.T) {
	ext := func(name string, data any) map[string]any {
		c := map[string]any{"kind": "extension", "extension": name}
		if data != nil {
			c["data"] = data
		}
		return c
	}
	cs := []contribapi.Contribution{
		fixtureEntry(t, objectpaneapi.PanelComponentKind, "known-view", map[string]any{"description": "a view"}),
		fixtureEntry(t, objectpaneapi.PanelComponentKind, "typed-view", map[string]any{"description": "a typed view", "input": "ktw-example:/data-type/typed-input"}),
		fixtureEntry(t, objectpaneapi.ObjectPanelKind, "good", map[string]any{"name": "Good", "content": ext("known-view", nil)}),
		fixtureEntry(t, objectpaneapi.ObjectPanelKind, "lost", map[string]any{"name": "Lost", "content": ext("no-such-view", nil)}),
		fixtureEntry(t, objectpaneapi.ObjectPanelKind, "typed-ok", map[string]any{"name": "Ok", "content": ext("typed-view", map[string]any{"title": "x"})}),
		fixtureEntry(t, objectpaneapi.ObjectPanelKind, "typed-bad", map[string]any{"name": "Bad", "content": ext("typed-view", map[string]any{"other": 1})}),
		fixtureEntry(t, objectpaneapi.ObjectPanelKind, "sourced", map[string]any{"name": "Sourced", "source": "ktw-example:a/b", "content": ext("typed-view", map[string]any{"other": 1})}),
	}
	check := func(schema string, body []byte) ([]string, error) {
		var m map[string]any
		_ = json.Unmarshal(body, &m)
		if _, ok := m["title"]; !ok {
			return []string{"title is required"}, nil
		}
		return nil, nil
	}
	got := panelComponentFindings(cs, check)
	if len(got) != 2 {
		t.Fatalf("want 2 findings (lost, typed-bad), got %d:\n  %s", len(got), strings.Join(got, "\n  "))
	}
	all := strings.Join(got, "\n")
	for _, want := range []string{"ktw-example:/object-panel/lost", `"no-such-view"`, "ktw-example:/object-panel/typed-bad", "title is required"} {
		if !strings.Contains(all, want) {
			t.Errorf("findings do not name %s:\n%s", want, all)
		}
	}
}

// TestEveryShippedObjectPanelNamesAPublishedComponent: every shipped object
// panel mounting a named view names one a shipped pack publishes, and a
// static panel's data matches its view's input data-type.
func TestEveryShippedObjectPanelNamesAPublishedComponent(t *testing.T) {
	cs := shippedPackEntries(t)
	panels, named := 0, 0
	for _, c := range entriesOfKind(cs, objectpaneapi.ObjectPanelKind) {
		panels++
		var p objectpaneapi.PanelDecl
		if json.Unmarshal(c.Data(), &p) == nil && p.Content != nil && p.Content.Kind == "extension" {
			named++
		}
	}
	// The floor is what the packs of this copy ship (the open-source copy
	// carries fewer packs than the workspace): every panel file is read.
	if files := shippedPanelFiles(t); panels != files || files == 0 || named == 0 {
		t.Fatalf("census: read %d object panels of the %d panel files shipped, %d naming a view: the pack reader finds too little", panels, files, named)
	}
	if n := len(entriesOfKind(cs, objectpaneapi.PanelComponentKind)); n == 0 {
		t.Fatal("census: no panel-component entry read from the shipped packs")
	}
	svc := schemaService(t)
	registered := registerDataTypes(t, svc, cs)
	check := func(schema string, body []byte) ([]string, error) {
		if !registered[schema] {
			return []string{"the data-type is not shipped"}, nil
		}
		out, err := svc.ValidateAgainst(context.Background(), schemaapi.ValidateAgainstInput{URI: schema, Body: body})
		if err != nil {
			return nil, err
		}
		var msgs []string
		for _, d := range out.Diagnostics {
			msgs = append(msgs, d.Message)
		}
		if !out.Valid && len(msgs) == 0 {
			msgs = []string{"invalid"}
		}
		return msgs, nil
	}
	for _, f := range panelComponentFindings(cs, check) {
		t.Error(f)
	}
}

// guiExtensionsFile is the GUI's ui-components registry, from the repo root.
var guiExtensionsFile = filepath.FromSlash("app/ktw-gui/frontend/src/sections/workspace/RenderDefView.tsx")

// guiExtensionKey is one key of the registry's object literal, quoted or not,
// at its first indentation.
var guiExtensionKey = regexp.MustCompile(`^  '?([a-z][a-z0-9-]*)'?: \(`)

// guiExtensionViews reads the names the GUI registers in UI_EXTENSIONS.
func guiExtensionViews(src string) []string {
	var out []string
	in := false
	for _, line := range strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n") {
		if strings.HasPrefix(line, "const UI_EXTENSIONS") {
			in = true
			continue
		}
		if !in {
			continue
		}
		if line == "}" {
			break
		}
		if m := guiExtensionKey.FindStringSubmatch(line); m != nil {
			out = append(out, m[1])
		}
	}
	sort.Strings(out)
	return out
}

// componentDrift names each view registered on one side only.
func componentDrift(gui, published []string) []string {
	g, p := map[string]bool{}, map[string]bool{}
	for _, n := range gui {
		g[n] = true
	}
	for _, n := range published {
		p[n] = true
	}
	var out []string
	for _, n := range gui {
		if !p[n] {
			out = append(out, "the GUI registers the view "+n+", which no panel-component entry publishes")
		}
	}
	for _, n := range published {
		if !g[n] {
			out = append(out, "a panel-component entry publishes the view "+n+", which the GUI does not register")
		}
	}
	sort.Strings(out)
	return out
}

// TestGUIExtensionViewsEqualThePublishedPanelComponents: the names the GUI
// frontend registers as ui-components views equal the published
// panel-component entries; one more on either side fails, naming it.
func TestGUIExtensionViewsEqualThePublishedPanelComponents(t *testing.T) {
	src, err := os.ReadFile(filepath.Join(repoRoot(t), guiExtensionsFile))
	if err != nil {
		t.Fatalf("census: read the GUI registry: %v", err)
	}
	gui := guiExtensionViews(string(src))
	if len(gui) < 5 {
		t.Fatalf("census: read %d views from %s: the registry reader finds too little", len(gui), guiExtensionsFile)
	}
	var published []string
	for _, c := range entriesOfKind(shippedPackEntries(t), objectpaneapi.PanelComponentKind) {
		published = append(published, c.ID)
	}
	sort.Strings(published)
	for _, f := range componentDrift(gui, published) {
		t.Error(f)
	}
	if t.Failed() {
		t.Logf("GUI: %v\npublished: %v", gui, published)
	}
}
