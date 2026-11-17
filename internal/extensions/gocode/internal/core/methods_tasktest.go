// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/gocode/api"
)

// TasktestCreate authors an empty L2 scenario file at the requested
// path + writes a paired `test-case` item to the owning module so
// the verify aggregator can pick it up via the TCID convention.
//
// v1 behavior: scenario file gets the canonical front-matter +
// empty step list. The tester persona authors the actual steps
// after the verb returns (the verb gives them a scaffold to start
// from, the same way `module create --scaffold` does for Go code).
func (s *GocodeServiceImpl) TasktestCreate(ctx stdctx.Context, in api.TasktestCreateInput) (api.TasktestCreateOutput, error) {
	if in.UCID == "" {
		return api.TasktestCreateOutput{}, fmt.Errorf("%w: TasktestCreate: uc_id is required", api.ErrInvalidInput)
	}
	if in.ModuleID == "" {
		return api.TasktestCreateOutput{}, fmt.Errorf("%w: TasktestCreate: module_id is required", api.ErrInvalidInput)
	}

	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return api.TasktestCreateOutput{}, fmt.Errorf("TasktestCreate: resolve CWMService: %w", err)
	}

	// Resolve target directory. OutputDir defaults to a per-module
	// L2 location; module.code_packages doesn't carry the test root
	// directly so we synthesize one — the architect can rename later.
	outputDir := in.OutputDir
	if outputDir == "" {
		outputDir = filepath.Join("modules", "tests", "L2")
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return api.TasktestCreateOutput{}, fmt.Errorf("TasktestCreate: mkdir %s: %w", outputDir, err)
	}

	// Filename derives from the UC id, lowercased. The TCID
	// convention (`L2-uc<NNN>-<slug>`) is what verify uses to bind
	// test-cases back to UCs — keep them parallel.
	slug := strings.ToLower(strings.ReplaceAll(in.UCID, "_", "-"))
	tcid := "L2-" + slug + "-scaffold"
	path := filepath.Join(outputDir, tcid+".tasktest")

	scenario := tasktestScaffold(in.UCID, tcid)
	if err := os.WriteFile(path, []byte(scenario), 0o644); err != nil {
		return api.TasktestCreateOutput{}, fmt.Errorf("TasktestCreate: write %s: %w", path, err)
	}

	// Author the paired test-case item on the module.
	now := time.Now().UTC()
	tcBody := map[string]any{
		"tcid":            tcid,
		"layer":           string(api.UCLayerL2),
		"path":            path,
		"last_run_status": string(api.TestRunNotRun),
	}
	bodyBytes, err := json.Marshal(tcBody)
	if err != nil {
		return api.TasktestCreateOutput{}, fmt.Errorf("TasktestCreate: marshal test-case body: %w", err)
	}
	module, err := cwmapi.ModuleOfObjectID(in.ModuleID)
	if err != nil {
		return api.TasktestCreateOutput{}, fmt.Errorf("TasktestCreate: owning module: %w", err)
	}
	// tcid is already the authored, human test-case id — it IS the identity now.
	obj := cwmapi.NewRecordObject(module, tcid, "test-case", in.ModuleID, tcid, bodyBytes)
	itemOut, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: module, Object: obj})
	if err != nil {
		return api.TasktestCreateOutput{}, fmt.Errorf("TasktestCreate: ObjectCreate test-case: %w", err)
	}
	_ = now

	return api.TasktestCreateOutput{
		Path:       path,
		TestCaseID: itemOut.Object.ID,
	}, nil
}

// TasktestRun v1 reports `not-run` and surfaces a clear hint that
// running L2 scenarios needs the ktw-tasktest simulator integration,
// which is the ms08 simulator-wiring task. The verb is non-error so
// chains depending on it (verify) can still progress; the test-case
// stays at `not-run` until a real run happens via either:
//
//   - `tasktest run <path>` invoked outside this verb (manual)
//   - The future ktw-tasktest simulator-runner verb (ms08)
//
// The verb explicitly does NOT silently succeed — it returns a
// non-empty `Failure` field with the hint so callers (especially
// the tester persona) can react appropriately.
func (s *GocodeServiceImpl) TasktestRun(ctx stdctx.Context, in api.TasktestRunInput) (api.TasktestRunOutput, error) {
	if in.Path == "" {
		return api.TasktestRunOutput{}, fmt.Errorf("%w: TasktestRun: path is required", api.ErrInvalidInput)
	}
	if _, err := os.Stat(in.Path); err != nil {
		return api.TasktestRunOutput{
			Status:  api.TestRunHarnessError,
			Failure: fmt.Sprintf("scenario file not found: %s", in.Path),
		}, nil
	}
	return api.TasktestRunOutput{
		Status:  api.TestRunNotRun,
		Failure: "tasktest.Run is scaffolded but the L2 simulator runner is filed under ms08 simulator-wiring; run scenarios outside this verb for now",
	}, nil
}

// tasktestScaffold returns the canonical empty .tasktest file. Real
// `.tasktest` grammar lives in the existing `ktw-test` harness; this
// scaffold matches the YAML-front-matter+markdown-body shape that
// harness consumes.
func tasktestScaffold(ucID, tcid string) string {
	return `---
tcid: ` + tcid + `
layer: L2
verifies: ` + ucID + `
description: >-
  Scaffolded by gocode tasktest.Create. Author the L2 scenario by
  filling in the steps section below; the tester persona's
  gocode-test-author-l2 skill describes the shape.
---

# ` + tcid + `

## Setup

(describe initial state)

## Steps

(numbered steps; each invokes a tool + asserts outcome)

## Teardown

(restore state)
`
}
