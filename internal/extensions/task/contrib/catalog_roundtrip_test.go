// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"context"
	"encoding/json"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	manifestlocal "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/local"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
)

// A task published by the generic producer reads back from the catalog as the
// engine reads its file: its contracts from their own contributions (a flat
// task's and a folder task's, named by the task's name), the defaults the
// parser applies (kind, capability, session mode) applied by the reader.
func TestTasksAndTheirContractsReachTheCatalog(t *testing.T) {
	files := map[string]string{
		"tasks/greet.md":                    "---\nname: greet\ntype: agent\ndescription: d\nconfig:\n  capability: smart\n  mode: chat\n---\nSay hello.\n",
		"tasks/greet.input.dsl.yaml":        "name: greet-input\nroot: In\ntypes:\n  In:\n    type: object\n    fields:\n      who: {type: string}\n",
		"tasks/review/TASK.md":              "---\nname: code-review\ntype: agent\ndescription: d\nkind: object\ntypes: [module]\n---\nReview.\n",
		"tasks/review/TASK.output.dsl.yaml": "name: review-output\nroot: Out\ntypes:\n  Out:\n    type: object\n    fields:\n      ok: {type: boolean}\n",
	}
	ctx := context.Background()
	m := &mbapi.Manifest{Provider: "ktw-test"}
	for _, p := range TaskProviders("ktw-test", loader.NewMapLoader(files), "tasks") {
		entries, err := p.Enumerate(ctx)
		if err != nil {
			t.Fatal(err)
		}
		m.Contributions = append(m.Contributions, entries...)
	}
	cat := manifestlocal.NewMemoryCatalog([]*mbapi.Manifest{m}, []string{"ktw-test"})

	// The contracts, by the task's name — a folder task's TASK.output.dsl.yaml
	// is code-review's, not "TASK"'s.
	in, out, err := taskapi.TaskContracts(ctx, cat, "ktw-test", "greet")
	if err != nil || len(out) != 0 {
		t.Fatalf("greet contracts: out=%s err=%v; want its input only", out, err)
	}
	var doc map[string]any
	if err := json.Unmarshal(in, &doc); err != nil || doc["name"] != "greet-input" {
		t.Fatalf("greet input = %s, %v; want greet.input.dsl.yaml as JSON", in, err)
	}
	in, out, err = taskapi.TaskContracts(ctx, cat, "ktw-test", "code-review")
	if err != nil || len(in) != 0 || len(out) == 0 {
		t.Fatalf("code-review contracts: in=%s out=%s err=%v; want its output only", in, out, err)
	}

	// The defaults, applied by the reader.
	bodies := map[string]mbapi.TaskBody{}
	for _, c := range m.Contributions {
		if c.Type != mbapi.ResourceTypeTask {
			continue
		}
		var b mbapi.TaskBody
		if err := json.Unmarshal(c.Data(), &b); err != nil {
			t.Fatal(err)
		}
		bodies[c.ID] = b
	}
	greet := taskapi.TaskSummaryFromCatalog("ktw-test", bodies["greet"])
	if greet.Kind != taskapi.KindGlobal || greet.Capability != "smart" || greet.SessionMode != "chat" {
		t.Errorf("greet summary = kind %q, capability %q, session %q; want global, smart (from config), chat (config.mode)", greet.Kind, greet.Capability, greet.SessionMode)
	}
	if review := taskapi.TaskSummaryFromCatalog("ktw-test", bodies["code-review"]); review.Kind != taskapi.KindObject {
		t.Errorf("code-review kind = %q, want object as written", review.Kind)
	}
	if bodies["greet"].Body != "Say hello." {
		t.Errorf("greet body = %q, want the markdown folded into data", bodies["greet"].Body)
	}
	if _, layer := contribapi.Contribution(m.Contributions[0]).Layers["body"]; layer {
		t.Error("the body is folded into data, not a layer of its own")
	}
}

// A task's translations (<stem>.i18n.yaml beside it) are its translations
// layer, as every other kind's (carries):
// the hub's translation bundle reads a translated task description.
func TestATasksTranslationsReachTheCatalog(t *testing.T) {
	files := map[string]string{
		"tasks/greet.md":        "---\nname: greet\ntype: agent\ndescription: Say hello\n---\nSay hello.\n",
		"tasks/greet.i18n.yaml": "fr:\n  description: Dire bonjour\n",
	}
	entries, err := TaskProviders("ktw-test", loader.NewMapLoader(files), "tasks")[0].Enumerate(context.Background())
	if err != nil || len(entries) != 1 {
		t.Fatalf("Enumerate = %v, %v; want the task alone (its translations file is not a task)", entries, err)
	}
	ref, ok := entries[0].Layers[contribapi.LayerTranslations]
	if !ok {
		t.Fatalf("layers = %v; want a translations layer", entries[0].Layers)
	}
	var tr map[string]map[string]any
	if err := json.Unmarshal(ref.Inline, &tr); err != nil || tr["fr"]["description"] != "Dire bonjour" {
		t.Fatalf("translations = %s, %v; want the .i18n.yaml as JSON", ref.Inline, err)
	}
}
