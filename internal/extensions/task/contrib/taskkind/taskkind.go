// SPDX-License-Identifier: Apache-2.0

// Package taskkind is the `task` kind as the task pack declares it: its
// contribution-type (task.yaml, the schema provider-relative) and its
// data-type, the shape of mbapi.TaskBody. A leaf: it imports the leaf shape
// and the Go-type helper only, so the built-in kind reader
// (internal/core/contribution/builtin) can import it without a cycle.
package taskkind

import (
	_ "embed"

	"github.com/katyoti/katyoti-fwk/internal/core/contribution/builtin/leaf"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	manifestcontrib "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/contrib"
)

//go:embed task.yaml
var declaration []byte

// Leaf is the task kind's declaration: its YAML and its schema.
func Leaf() leaf.Declaration {
	return leaf.Declaration{
		Kind: mbapi.ResourceTypeTask,
		YAML: declaration,
		Schema: manifestcontrib.GoTypeOf[mbapi.TaskBody]("task", "2.1.0", "Task",
			"A runnable task type (TASK.md): a strategy reference + strategy-specific\n"+
				"config, its body, and how it surfaces. The 3-level model is Strategy (Go) -\n"+
				"Task Type (TASK.md) - Instance (runtime). `config` is decoded by the\n"+
				"strategy. The input contract is declared, not enforced.\n"),
	}
}
