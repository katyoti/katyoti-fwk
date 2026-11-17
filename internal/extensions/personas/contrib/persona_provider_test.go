// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// A persona read by the generic producer decodes as the PersonaBody the
// persona library reads: its frontmatter with the JSON names, its body folded
// as `body`, an augmentation's scope (applies_to) kept; a task file of another
// category is not a persona.
func TestPersonaProvider_ReadsWhatThePersonaLibraryDecodes(t *testing.T) {
	files := map[string]string{
		"embed/personas/coder/TASK.md":    "---\nname: coder\ntype: agent-loop\ncategory: persona\ndescription: writes code\niconName: code\nicon: \"🛠\"\nbind_mode: object\ntypes: [module]\nskills: [go]\nconfig:\n  maxTurns: 30\n---\nWrite the code.\n",
		"embed/personas/coder-go/TASK.md": "---\nname: coder-go\ntype: agent-loop\ncategory: persona\naugments: coder\nadvertised: false\napplies_to:\n  files: [\"**/*.go\"]\n  modules: [billing]\n---\nGo specifics\n",
		"embed/personas/plain/TASK.md":    "---\nname: plain\ntype: agent-loop\ncategory: chat\n---\nnot a persona\n",
	}
	p := PersonaProvider("ktw-test", loader.NewMapLoader(files), "embed/personas")
	entries, err := p.Enumerate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	bodies := map[string]mbapi.PersonaBody{}
	for _, e := range entries {
		var b mbapi.PersonaBody
		if err := json.Unmarshal(e.Data(), &b); err != nil {
			t.Fatalf("%s: decode PersonaBody: %v", e.ID, err)
		}
		bodies[e.ID] = b
	}
	if len(bodies) != 2 {
		t.Fatalf("personas = %v; want coder and coder-go, the chat task left out", bodies)
	}
	coder := bodies["coder"]
	if coder.Body != "Write the code." || coder.IconName != "code" || coder.Icon != "🛠" || coder.BindMode != "object" ||
		len(coder.Types) != 1 || len(coder.Skills) != 1 || string(coder.Config) != `{"maxTurns":30}` {
		t.Errorf("coder = %+v; want its frontmatter and its body as the library reads them", coder)
	}
	aug := bodies["coder-go"]
	if aug.Augments != "coder" || aug.AppliesTo == nil || len(aug.AppliesTo.Files) != 1 || aug.AppliesTo.Modules[0] != "billing" {
		t.Errorf("coder-go = %+v; want an augmentation of coder carrying its scope", aug)
	}
	if aug.Advertised == nil || *aug.Advertised {
		t.Errorf("an augmentation says advertised: false, so no listing shows it beside its baseline")
	}
}
