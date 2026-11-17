// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/katyoti/katyoti-fwk/internal/core/application"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// AN INCREMENT THAT CHANGES SCREENS PLANS ITS DISPLAY SWITCH FIRST (
// ): a proposal adding a feature to an increment in a
// module that draws a screen is refused at reconcile unless that increment
// holds its display switch - a feature-add of the same delta with the role
// display-switch aimed at it, or a feature already tagged display-switch that
// it targets. Once delivered, the switch is tagged and every screen feature the
// delta aimed at the increment depends on it: it is built first. A feature-add
// with the role no-screen changes no screen although its module draws one: the
// guard does not count it, and it is tagged no-screen once delivered (RQ-420).

// incrementOfOp is the increment an operation aims what it adds at: its own,
// or - for an increment's own proposal - that increment.
func incrementOfOp(op api.DeltaOp, owner string) string {
	if op.Increment != "" {
		return op.Increment
	}
	return owner
}

// checkScreenSwitch refuses a delta that adds screen features to an increment
// holding no display switch.
func checkScreenSwitch(ctx context.Context, cwm cwmapi.CWMService, request string, d []api.DeltaOp) error {
	owner := ""
	if o, _, err := loadAny(ctx, cwm, request); err == nil && o.Type == api.IncrementType {
		owner = o.ID
	}
	screens := map[string][]int{} // increment -> the screen feature-adds aimed at it
	switches := map[string]bool{}
	for i, op := range d {
		if op.Op != api.OpFeatureAdd {
			continue
		}
		inc := incrementOfOp(op, owner)
		if inc == "" {
			continue
		}
		if op.Role == api.RoleDisplaySwitch {
			switches[inc] = true
			continue
		}
		if op.Role == api.RoleNoScreen {
			continue
		}
		if screenModule(ctx, cwm, op.Module) {
			screens[inc] = append(screens[inc], i)
		}
	}
	var problems []string
	for inc, ops := range screens {
		if switches[inc] || (!isRef(inc) && holdsDisplaySwitch(ctx, cwm, inc)) {
			continue
		}
		var at []string
		for _, i := range ops {
			at = append(at, fmt.Sprintf("delta[%d]", i))
		}
		problems = append(problems, fmt.Sprintf("%s add screen features to %s, which holds no display switch: add first a feature-add with role %q aimed at it - the feature that puts its new screens behind a switch, turned on at its approved delivery",
			strings.Join(at, ", "), inc, api.RoleDisplaySwitch))
	}
	if len(problems) == 0 {
		return nil
	}
	slices.Sort(problems)
	return invalid("%s", strings.Join(problems, "\n"))
}

// screenModule: the module's code draws a screen - a code package is the GUI or
// the display runtime or library, or ships object panels (embed/cwm/
// object-panels: the panes and pages a pack renders, planification's Delivery
// view among them).
func screenModule(ctx context.Context, cwm cwmapi.CWMService, module string) bool {
	if module == "" {
		return false
	}
	m, err := loadObject(ctx, cwm, module)
	if err != nil {
		return false
	}
	code, _, _ := cwmapi.DecodeCapability[struct {
		Packages []string `json:"code_packages"`
	}](m, "code")
	root := moduleRoot(ctx, cwm, module)
	return slices.ContainsFunc(code.Packages, func(pkg string) bool { return isScreenPath(pkg) || shipsPanels(root, pkg) })
}

// moduleRoot is the project root a module's code lives under: the root of the
// project CWM serves the module from, never the process-wide one when the
// module resolves - a world of its own is probed in its own tree
func moduleRoot(ctx context.Context, cwm cwmapi.CWMService, module string) string {
	if r, err := cwm.ObjectResolve(ctx, cwmapi.ObjectResolveInput{Ref: module}); err == nil && r.ProjectRoot != "" {
		return r.ProjectRoot
	}
	return application.ProjectRoot()
}

// panelsShipped caches, per project root and code package, whether it ships
// object panels.
var panelsShipped sync.Map

// shipsPanels: a folder object-panels under an embed/cwm tree of the package,
// in the project rooted at root.
func shipsPanels(root, pkg string) bool {
	key := filepath.Join(root, filepath.FromSlash(pkg))
	if v, ok := panelsShipped.Load(key); ok {
		return v.(bool)
	}
	found := false
	_ = filepath.WalkDir(key, func(p string, d fs.DirEntry, err error) error {
		if err != nil || found {
			return filepath.SkipDir
		}
		if d.IsDir() {
			switch d.Name() {
			case "node_modules", "testdata", ".git":
				return filepath.SkipDir
			case "object-panels":
				if filepath.Base(filepath.Dir(p)) == "cwm" {
					found = true
					return filepath.SkipAll
				}
			}
		}
		return nil
	})
	panelsShipped.Store(key, found)
	return found
}

// holdsDisplaySwitch: a feature tagged display-switch targets the increment.
func holdsDisplaySwitch(ctx context.Context, cwm cwmapi.CWMService, inc string) bool {
	ids, _ := workOf(ctx, cwm, inc)
	for _, id := range ids {
		if o, err := loadObject(ctx, cwm, id); err == nil {
			if _, ok := o.Capabilities[api.CapDisplaySwitch]; ok {
				return true
			}
		}
	}
	return false
}

// orderScreenSwitch, once a delta is delivered: the feature added with the role
// display-switch is tagged, every screen feature the delta aimed at the same
// increment depends on it, and a feature added with the role no-screen is
// tagged no-screen.
func orderScreenSwitch(ctx context.Context, cwm cwmapi.CWMService, owner string, d []api.DeltaOp, applied []api.Applied) {
	made := map[int]string{}
	for _, x := range applied {
		made[x.Op] = x.ID
	}
	switchOf := map[string]string{}
	for i, op := range d {
		if op.Op == api.OpFeatureAdd && op.Role == api.RoleNoScreen && made[i] != "" {
			if _, err := cwm.ObjectAddCapability(ctx, cwmapi.ObjectAddCapabilityInput{ID: made[i], Capability: api.CapNoScreen}); err != nil {
				slog.Warn("planification: a no-screen feature was not tagged", "feature", made[i], "error", err)
			}
		}
		if op.Op == api.OpFeatureAdd && op.Role == api.RoleDisplaySwitch && made[i] != "" {
			inc := resolveRef(incrementOfOp(op, owner), applied)
			switchOf[inc] = made[i]
			if _, err := cwm.ObjectAddCapability(ctx, cwmapi.ObjectAddCapabilityInput{ID: made[i], Capability: api.CapDisplaySwitch}); err != nil {
				slog.Warn("planification: the display switch was not tagged", "feature", made[i], "error", err)
			}
		}
	}
	for i, op := range d {
		if op.Op != api.OpFeatureAdd || op.Role == api.RoleDisplaySwitch || op.Role == api.RoleNoScreen || made[i] == "" {
			continue
		}
		sw := switchOf[resolveRef(incrementOfOp(op, owner), applied)]
		if sw == "" || !screenModule(ctx, cwm, op.Module) {
			continue
		}
		if err := addEdge(ctx, cwm, made[i], api.EdgeDependsOn, sw); err != nil {
			slog.Warn("planification: a screen feature was not ordered after its display switch", "feature", made[i], "switch", sw, "error", err)
		}
	}
}
