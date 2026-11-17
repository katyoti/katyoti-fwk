// SPDX-License-Identifier: Apache-2.0

package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// kindCatalog answers GetContributionsByType from a fixed list; any other
// Catalog call panics, so the tool proves it reads metadata only.
type kindCatalog struct {
	mbapi.Catalog
	entries []contribapi.Contribution
}

func (c kindCatalog) GetContributionsByType(_ context.Context, t string) ([]contribapi.Contribution, error) {
	var out []contribapi.Contribution
	for _, e := range c.entries {
		if e.Type == t {
			out = append(out, e)
		}
	}
	return out, nil
}

func entry(provider, typ, id, name, desc string, tags ...string) contribapi.Contribution {
	return contribapi.Contribution{Provider: provider, Type: typ, ID: id,
		Metadata: &contribapi.ContributionMeta{Name: name, Description: desc, Hashtags: tags}}
}

// TestAgentFindsRecipeByIntent is the G-005 probe of modules/guide: asked
// how to do something, the agent's list-resources tool finds the guide
// recipe by the words a person would use (its hashtags), returns its URI
// first, and leaves out recipes that do not match and resources of another
// kind that do.
func TestAgentFindsRecipeByIntent(t *testing.T) {
	app := application.New(application.Config())
	application.PutService[mbapi.Catalog](app, "default", kindCatalog{entries: []contribapi.Contribution{
		entry("ktw-univers", "guide-recipe", "add-a-capability", "Add a capability to my universe", "Install a sidecar and wire it.", "capability", "add capability", "install sidecar", "marketplace"),
		entry("ktw-univers", "guide-recipe", "answer-the-inbox", "Answer a question in the Inbox", "Answer a request.", "inbox", "answer"),
		entry("ktw-univers", "guide-recipe", "apply-a-config-set", "Apply a config-set", "Apply keys to your universe.", "config-set", "apply"),
		entry("ktw-agents", "skill", "sidecar-install", "Install sidecar", "A skill, not a recipe.", "install sidecar"),
	}})
	ctx := application.WithApplication(context.Background(), app)

	res, err := NewListResourcesTool().ToolCall(ctx, json.RawMessage(`{"kind":"guide-recipe","words":"install sidecar"}`))
	if err != nil {
		t.Fatal(err)
	}
	var out ListResourcesOutput
	if err := json.Unmarshal([]byte(res.ResultString()), &out); err != nil {
		t.Fatalf("result %q: %v", res.ResultString(), err)
	}
	if len(out.Resources) != 1 || out.Resources[0].URI != "ktw-univers:/guide-recipe/add-a-capability" {
		t.Fatalf("install sidecar → %+v, want exactly the add-a-capability recipe", out.Resources)
	}

	// Ranking: a word in the hashtags outranks one only in the description.
	hits := SearchResources([]contribapi.Contribution{
		entry("p", "guide-recipe", "b", "Keys", "How to apply them.", "keys"),
		entry("p", "guide-recipe", "a", "Apply a config-set", "Keys.", "apply"),
	}, "apply", 0)
	if len(hits) != 2 || hits[0].URI != "p:/guide-recipe/a" {
		t.Errorf("ranking: %+v", hits)
	}
	// An entry published with no metadata is listed with no words, never a crash.
	if got := SearchResources([]contribapi.Contribution{{Provider: "p", Type: "guide-recipe", ID: "bare"}}, "", 0); len(got) != 1 {
		t.Errorf("entry without metadata: %+v", got)
	}
	if got := SearchResources(nil, "anything", 0); len(got) != 0 {
		t.Errorf("empty catalog: %+v", got)
	}
}

// layerCatalog serves one layer through the catalog's layer lane; any other
// Catalog call panics.
type layerCatalog struct {
	mbapi.Catalog
	uri, layer string
	content    []byte
}

func (c layerCatalog) OpenLayer(_ context.Context, uri, layer string) (io.ReadCloser, contribapi.LayerRef, error) {
	if uri != c.uri || layer != c.layer {
		return nil, contribapi.LayerRef{}, contribapi.ErrLayerNotDeclared{URI: uri, Layer: layer}
	}
	return io.NopCloser(bytes.NewReader(c.content)), contribapi.LayerRef{Hash: contribapi.HashContent(c.content), Size: int64(len(c.content))}, nil
}

// read-resource reads a named layer when asked — how the agent reads a guide
// recipe's translations to answer in the person's language.
func TestReadResourceReadsANamedLayer(t *testing.T) {
	const uri = "ktw-hub:/guide-recipe/add-a-capability"
	tr := []byte(`{"fr":{"name":"Ajouter une capacité"}}`)
	app := application.New(application.Config())
	application.PutService[mbapi.Catalog](app, "default", layerCatalog{uri: uri, layer: "translations", content: tr})
	ctx := application.WithApplication(context.Background(), app)

	res, err := NewReadResourceTool().ToolCall(ctx, json.RawMessage(`{"uri":"`+uri+`","layer":"translations"}`))
	if err != nil {
		t.Fatal(err)
	}
	var out ReadResourceOutput
	if err := json.Unmarshal([]byte(res.ResultString()), &out); err != nil || out.Content != string(tr) {
		t.Fatalf("translations layer: %q (%v)", res.ResultString(), err)
	}
	if _, err := NewReadResourceTool().ToolCall(ctx, json.RawMessage(`{"uri":"`+uri+`","layer":"nope"}`)); err == nil {
		t.Error("an undeclared layer was read")
	}
}
