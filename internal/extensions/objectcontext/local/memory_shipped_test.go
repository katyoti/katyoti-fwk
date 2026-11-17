// SPDX-License-Identifier: Apache-2.0

package local

import (
	"os"
	"strings"
	"testing"
	"testing/fstest"

	agentapi "github.com/katyoti/katyoti-fwk/internal/extensions/agent/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/objectcontext/api"
	"github.com/katyoti/katyoti-fwk/internal/test/cwmapp"
)

// THE SHIPPED MEMORY (AI memory handoff): the ktw-agents pack's
// memory capability, loaded as shipped, attaches to every type ONE named file,
// `working`, at memory/${id}.md; a chat on the object receives it as the
// "Working memory" section — "(empty)" while nothing is written, so the agent
// does not go looking for it — and what the last session wrote once it is.

const shippedMemory = "../../../../app/ktw-agents/embed/cwm/capabilities/memory.yaml"

func TestTheShippedMemoryIsOneWorkingFileHandedToTheChat(t *testing.T) {
	data, err := os.ReadFile(shippedMemory)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cwm := cwmapp.New(t,
		cwmapp.Pack(fstest.MapFS{
			"capabilities/container.yaml": {Data: []byte("name: container\n")},
			"capabilities/memory.yaml":    {Data: data},
			"object-types/module.yaml":    {Data: []byte("name: module\ncapabilities: [container]\n")},
			"object-types/goal.yaml":      {Data: []byte("name: goal\ncapabilities: [container]\n")},
		}),
		ExtensionWith(api.ObjectContextConfig{}),
	)
	cwmapp.Module(t, ctx, cwm, "billing")
	g := cwmapi.Object{ID: "billing.g1", Type: "goal", Name: "Invoices go out on time", Parent: "billing"}
	if _, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: "billing", Object: g}); err != nil {
		t.Fatal(err)
	}

	list, err := cwm.ObjectNamedFileList(ctx, cwmapi.ObjectNamedFileListInput{ID: "billing.g1"})
	if err != nil {
		t.Fatal(err)
	}
	var memory []cwmapi.ObjectNamedFile
	for _, f := range list.Files {
		if f.Capability == "memory" {
			memory = append(memory, f)
		}
	}
	if len(memory) != 1 || memory[0].Name != "working" || memory[0].Path != "memory/billing.g1.md" {
		t.Fatalf("the shipped memory must declare ONE file, working, at memory/${id}.md: %+v", memory)
	}

	chat := func() string {
		plugins, err := create(agentapi.WithPluginContext(ctx, map[string]any{agentapi.PluginContextTarget: "billing.g1"}), nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		return systemOf(plugins)
	}
	if sys := chat(); !strings.Contains(sys, "## Working memory\n"+"(empty)") {
		t.Fatalf("an unwritten memory must reach the chat as (empty):\n%s", sys)
	}
	if _, err := cwm.ObjectNamedFileWrite(ctx, cwmapi.ObjectNamedFileWriteInput{ID: "billing.g1", Capability: "memory", Name: "working", Content: []byte("Next: send the August reminders.")}); err != nil {
		t.Fatal(err)
	}
	if sys := chat(); !strings.Contains(sys, "## Working memory\nNext: send the August reminders.") || strings.Contains(sys, "## Working memory\n(empty)") {
		t.Fatalf("the written memory must reach the chat in place of (empty):\n%s", sys)
	}
}
