// SPDX-License-Identifier: Apache-2.0

package template

import (
	"bytes"
	"context"
	"fmt"
	tmpl "text/template"

	"github.com/katyoti/katyoti-fwk/internal/core/strategies"
	taskapi "github.com/katyoti/katyoti-fwk/internal/extensions/task/api"
	engine "github.com/katyoti/katyoti-fwk/internal/extensions/task/api/engine"
)

// MachineStrategyDef registers "template" in the MachineDomain.
var MachineStrategyDef = strategies.FuncStrategy[engine.MachineFactory](
	"template", "Template rendering (step)",
	func() (engine.MachineFactory, error) {
		return &templateFactory{}, nil
	},
)

type templateFactory struct{}

func (f *templateFactory) Name() string { return "template" }

// CreateMachine reads a task's request: its body is the template (else
// config `template`), its input the data. A bare map (a direct caller) gives
// both: `template` and `data`.
func (f *templateFactory) CreateMachine(_ context.Context, request any) (engine.Machine, error) {
	var templateStr string
	var data map[string]any
	switch r := request.(type) {
	case *taskapi.TaskExecRequest:
		templateStr = r.Body
		if cfg, _ := r.Config.(map[string]any); templateStr == "" && cfg != nil {
			templateStr, _ = cfg["template"].(string)
		}
		data = r.Input
	case map[string]any:
		templateStr, _ = r["template"].(string)
		data, _ = r["data"].(map[string]any)
	}
	if templateStr == "" {
		return nil, fmt.Errorf("template: no template (the task's body, or config 'template')")
	}
	if data == nil {
		data = make(map[string]any)
	}

	return engine.OneShotMachine("template", func(ctx context.Context) (any, error) {
		t, err := tmpl.New("template").Parse(templateStr)
		if err != nil {
			return nil, fmt.Errorf("template parse error: %w", err)
		}

		var buf bytes.Buffer
		if err := t.Execute(&buf, data); err != nil {
			return nil, fmt.Errorf("template render error: %w", err)
		}

		return buf.String(), nil
	}), nil
}
