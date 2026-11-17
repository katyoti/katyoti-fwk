// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/gocode/api"
)

// UCCreate authors a `use-case` item attached to a module node. The
// specifier persona is the primary caller. Status_cache starts at
// `no-test` (no test-cases verify it yet) and only the `verify`
// verb is allowed to mutate it from there — see UCSetStatus.
func (s *GocodeServiceImpl) UCCreate(ctx stdctx.Context, in api.UCCreateInput) (api.UCCreateOutput, error) {
	if in.NodeID == "" {
		return api.UCCreateOutput{}, fmt.Errorf("%w: UCCreate: node_id is required", api.ErrInvalidInput)
	}
	if in.UCID == "" {
		return api.UCCreateOutput{}, fmt.Errorf("%w: UCCreate: uc_id is required", api.ErrInvalidInput)
	}
	if in.Title == "" {
		return api.UCCreateOutput{}, fmt.Errorf("%w: UCCreate: title is required", api.ErrInvalidInput)
	}

	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return api.UCCreateOutput{}, fmt.Errorf("UCCreate: resolve CWMService: %w", err)
	}

	uc := api.UseCase{
		NodeID:       in.NodeID,
		UCID:         in.UCID,
		Title:        in.Title,
		Body:         in.BodyMD,
		Severity:     in.Severity,
		StatusCache:  api.UCStatusNoTest,
		LayerTargets: in.LayerTargets,
	}
	body, err := json.Marshal(ucBody(uc))
	if err != nil {
		return api.UCCreateOutput{}, fmt.Errorf("UCCreate: marshal body: %w", err)
	}

	// Identity collapsed to the authored id (2026-09-14): the use-case IS
	// <module>.<UC-NNN>. The uuid-plus-uc_id pair is gone, so nothing has to be
	// kept in sync and the filename says which UC it is.
	module, err := cwmapi.ModuleOfObjectID(in.NodeID)
	if err != nil {
		return api.UCCreateOutput{}, fmt.Errorf("UCCreate: owning module: %w", err)
	}
	obj := cwmapi.NewRecordObject(module, in.UCID, "use-case", in.NodeID, in.Title, body)
	obj.Body = in.BodyMD // the DoD prose is the object's body
	setSeverity(&obj, in.Severity)
	createOut, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: module, Object: obj})
	if err != nil {
		return api.UCCreateOutput{}, fmt.Errorf("UCCreate: ObjectCreate: %w", err)
	}
	uc.ID = createOut.Object.ID

	return api.UCCreateOutput{UseCase: uc}, nil
}

// UCUpdate patches title / severity / body / layer_targets on an
// existing use-case item. Pointer fields (*string, *UCSeverity) use
// omit-to-leave semantics so callers patch sparsely. Status_cache
// is NOT patchable here — that's UCSetStatus's exclusive territory.
func (s *GocodeServiceImpl) UCUpdate(ctx stdctx.Context, in api.UCUpdateInput) (api.UCUpdateOutput, error) {
	if in.ID == "" {
		return api.UCUpdateOutput{}, fmt.Errorf("%w: UCUpdate: id is required", api.ErrInvalidInput)
	}

	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return api.UCUpdateOutput{}, fmt.Errorf("UCUpdate: resolve CWMService: %w", err)
	}

	// Read the current object to preserve other typed fields.
	module, err := cwmapi.ModuleOfObjectID(in.ID)
	if err != nil {
		return api.UCUpdateOutput{}, fmt.Errorf("UCUpdate: owning module: %w", err)
	}
	getOut, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: in.ID})
	if err != nil {
		return api.UCUpdateOutput{}, fmt.Errorf("UCUpdate: ObjectGet: %w", err)
	}
	content := getOut.Object
	var current api.UseCase
	if raw := content.FieldsJSON(); len(raw) > 0 {
		_ = json.Unmarshal(raw, &current)
	}
	current.ID = getOut.Object.ID
	current.NodeID = getOut.Object.ParentID()
	current.Title = content.Name
	current.Body = content.Body // the DoD prose is the object's body
	current.Tags = content.Tags
	current.Severity = severityOf(content)
	if _, local, err := cwmapi.SplitObjectID(content.ID); err == nil {
		current.UCID = local
	}

	// Apply patches.
	if in.Title != nil {
		current.Title = *in.Title
	}
	if in.Severity != nil {
		current.Severity = *in.Severity
	}
	if in.BodyMD != nil {
		current.Body = *in.BodyMD
	}
	if in.LayerTargets != nil {
		current.LayerTargets = in.LayerTargets
	}
	body, err := json.Marshal(ucBody(current))
	if err != nil {
		return api.UCUpdateOutput{}, fmt.Errorf("UCUpdate: marshal body: %w", err)
	}
	// Update replaces the object wholesale, so carry the read object forward and
	// patch only what changed — re-building it from scratch would silently drop
	// any capability this verb does not know about.
	updated := getOut.Object
	updated.Name, updated.Body = current.Title, current.Body
	updated.SetFieldsJSON(body)
	setSeverity(&updated, current.Severity)
	if _, err := cwm.ObjectUpdate(ctx, cwmapi.ObjectUpdateInput{Module: module, Object: updated, Revision: getOut.Revision}); err != nil {
		return api.UCUpdateOutput{}, fmt.Errorf("UCUpdate: ObjectUpdate: %w", err)
	}

	return api.UCUpdateOutput{UseCase: current}, nil
}

// ucBody returns the use-case type's fields. Drops what the object core holds
// (id, title, tags, the DoD prose), what the checkable capability holds (the
// severity), the timestamps and the relations (the `deferred-by` edge).
func ucBody(uc api.UseCase) map[string]any {
	m := map[string]any{
		"status_cache": uc.StatusCache,
	}
	if uc.LastVerifiedAt != nil {
		m["last_verified_at"] = uc.LastVerifiedAt.Format(time.RFC3339)
	}
	if len(uc.LayerTargets) > 0 {
		m["layer_targets"] = uc.LayerTargets
	}
	return m
}

// severityOf is how much a use case matters: its checkable capability's.
func severityOf(o cwmapi.Object) api.UCSeverity {
	s, _ := o.Capabilities["checkable"]["severity"].(string)
	return api.UCSeverity(s)
}

// setSeverity writes the severity into the checkable group, keeping its other
// fields.
func setSeverity(o *cwmapi.Object, sev api.UCSeverity) {
	if sev == "" {
		return
	}
	if o.Capabilities == nil {
		o.Capabilities = map[string]map[string]any{}
	}
	g := map[string]any{}
	for k, v := range o.Capabilities["checkable"] {
		g[k] = v
	}
	g["severity"] = string(sev)
	o.Capabilities["checkable"] = g
}
