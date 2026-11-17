// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/extensions/code/api"
	projection "github.com/katyoti/katyoti-fwk/internal/extensions/code/cwmprojection"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// CharterPublish composes charter.md WHOLE from the module's objects —
// authored prose (charter object) + every projected section (UC table,
// DoD-per-UC, the Test plan, ODs, audit history, manifest snapshot) — and
// writes it to `<project>/<modules>/<m>/charter.md`. It is the one writer of
// that file: `module project` calls it, nothing splices into it.
//
// On DryRun=true, returns the rendered markdown via Output.Markdown
// without touching disk.
func (s *codeService) CharterPublish(ctx stdctx.Context, in api.CharterPublishInput) (api.CharterPublishOutput, error) {
	if in.ModuleID == "" {
		return api.CharterPublishOutput{}, fmt.Errorf("%w: CharterPublish: module_id is required", api.ErrInvalidInput)
	}

	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return api.CharterPublishOutput{}, fmt.Errorf("CharterPublish: resolve CWMService: %w", err)
	}

	inputs, err := projection.LoadCharterInputs(ctx, cwm, in.ModuleID)
	if err != nil {
		return api.CharterPublishOutput{}, fmt.Errorf("CharterPublish: load inputs: %w", err)
	}
	dir, err := moduleDir(ctx, cwm, in.ModuleID)
	if err != nil {
		return api.CharterPublishOutput{}, fmt.Errorf("CharterPublish: %w", err)
	}
	path := filepath.Join(dir, "charter.md")

	if in.DryRun {
		return api.CharterPublishOutput{
			Path:     path,
			Markdown: projection.BuildCharter(inputs),
		}, nil
	}

	if _, err := projection.WriteCharterMD(inputs, path); err != nil {
		return api.CharterPublishOutput{}, fmt.Errorf("CharterPublish: write %s: %w", path, err)
	}
	return api.CharterPublishOutput{Path: path}, nil
}

// ManifestWrite builds manifest.json WHOLE — the coverage block included —
// from the module's objects and writes it to
// `<project>/<modules>/<m>/manifest.json`. It is the one writer of that file.
//
// On DryRun=true, returns the rendered JSON via Output.JSON without touching
// disk.
func (s *codeService) ManifestWrite(ctx stdctx.Context, in api.ManifestWriteInput) (api.ManifestWriteOutput, error) {
	if in.ModuleID == "" {
		return api.ManifestWriteOutput{}, fmt.Errorf("%w: ManifestWrite: module_id is required", api.ErrInvalidInput)
	}

	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return api.ManifestWriteOutput{}, fmt.Errorf("ManifestWrite: resolve CWMService: %w", err)
	}

	inputs, err := projection.LoadManifestInputs(ctx, cwm, in.ModuleID)
	if err != nil {
		return api.ManifestWriteOutput{}, fmt.Errorf("ManifestWrite: load inputs: %w", err)
	}
	dir, err := moduleDir(ctx, cwm, in.ModuleID)
	if err != nil {
		return api.ManifestWriteOutput{}, fmt.Errorf("ManifestWrite: %w", err)
	}
	path := filepath.Join(dir, "manifest.json")

	manifest := projection.BuildManifest(inputs)
	if err := keepsSourcePaths(path, manifest, in.ModuleID); err != nil {
		return api.ManifestWriteOutput{}, fmt.Errorf("ManifestWrite: %w", err)
	}
	if in.DryRun {
		data, err := manifest.MarshalCanonicalJSON()
		if err != nil {
			return api.ManifestWriteOutput{}, fmt.Errorf("ManifestWrite: %w", err)
		}
		return api.ManifestWriteOutput{Path: path, JSON: string(data)}, nil
	}
	if _, err := projection.WriteManifestJSON(manifest, path); err != nil {
		return api.ManifestWriteOutput{}, fmt.Errorf("ManifestWrite: write %s: %w", path, err)
	}
	return api.ManifestWriteOutput{Path: path}, nil
}

// keepsSourcePaths refuses a manifest that would drop the source_paths the
// written one lists. source_paths is projected from the module capability's
// `code_packages`; a module object that does not declare them yet would
// otherwise erase, on --apply, the packages `module verify` runs in.
func keepsSourcePaths(path string, next projection.Manifest, moduleID string) error {
	if len(next.SourcePaths) > 0 {
		return nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil // no manifest written yet: nothing to drop
	}
	var current struct {
		SourcePaths []string `json:"source_paths"`
	}
	if err := json.Unmarshal(raw, &current); err != nil || len(current.SourcePaths) == 0 {
		return nil
	}
	return fmt.Errorf("%s lists source_paths %v but module %s declares no %s — "+
		"declare them on the module object (capabilities.module.%s) before projecting",
		path, current.SourcePaths, moduleID, api.CodePackagesField, api.CodePackagesField)
}

// moduleDir is the module's folder in the project the CWM service serves:
// its project root (ObjectResolve answers it) + the modules folder the project
// object declares + the module's name. The projections land beside the
// objects they are rendered from.
func moduleDir(ctx stdctx.Context, cwm cwmapi.CWMService, moduleID string) (string, error) {
	module, err := cwmapi.ModuleOfObjectID(moduleID)
	if err != nil {
		return "", fmt.Errorf("owning module of %s: %w", moduleID, err)
	}
	res, err := cwm.ObjectResolve(ctx, cwmapi.ObjectResolveInput{Ref: moduleID})
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", moduleID, err)
	}
	root := res.ProjectRoot
	if root == "" {
		return "", fmt.Errorf("the CWM service answers no project root for %s", moduleID)
	}
	modules := cwmapi.DefaultModulesDir
	if res.Project != "" {
		if p, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: res.Project, ID: res.Project}); err == nil {
			modules = p.Object.ModulesDir()
		}
	}
	return filepath.Join(root, modules, module), nil
}
