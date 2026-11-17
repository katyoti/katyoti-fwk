// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
)

// SourceTab is one of an opened source's three sub-tabs as a page region
// , shown under a holder carrying source-sub-tabs, whatever its
// connector: take - what the source takes, the connector's generic picker
// bound to the source's module with its selection and « Appliquer », the
// proposal region for a connector proposing from the holder's keywords
// ; read -
// « Contenu lu », the source's detail and each item it brought with its
// state; settings - « Réglages », how it searches and reads, edited in place
// (mock-up v6), then the connector's location form for a
// connector without a picker.
func (s *Service) SourceTab(ctx context.Context, in api.SourceTabInput) (api.ConnectorPanelOutput, error) {
	if strings.TrimSpace(in.ID) == "" {
		return api.ConnectorPanelOutput{}, fmt.Errorf("%w: sources.SourceTab: id required", cwmapi.ErrInvalidInput)
	}
	switch in.Tab {
	case api.SourceTabTake, api.SourceTabRead, api.SourceTabSettings:
	default:
		return api.ConnectorPanelOutput{}, fmt.Errorf("%w: sources.SourceTab: tab %q is none of %s, %s, %s", cwmapi.ErrInvalidInput, in.Tab, api.SourceTabTake, api.SourceTabRead, api.SourceTabSettings)
	}
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return api.ConnectorPanelOutput{}, fmt.Errorf("sources: resolve CWMService: %w", err)
	}
	detail, err := s.sourceDetail(ctx, cwm, in.ID)
	if err != nil {
		return api.ConnectorPanelOutput{}, err
	}
	title := fmt.Sprint(detail["label"])
	if in.Tab == api.SourceTabRead {
		// Grouped by kind when the source brings more than its unit
		// : the applications picked, then a table per
		// kind its connector types.
		tree := sourceDetailTree(detail, false)
		if src, module, err := sourceFields(ctx, cwm, in.ID); err == nil {
			if c, err := s.connector(ctx, src.Connector); err == nil {
				root := projectRoot(ctx, cwm, module)
				if groups := s.groupRead(ctx, detail, c, src.IDColumns, root); len(groups) > 0 {
					tree = readGroupsTree(detail, groups)
				} else if lead := s.unitLead(ctx, c, src.IDColumns); len(lead) > 0 {
					// The unit alone keeps its one table, led by its id
					// column.
					items, _ := detail["items"].([]any)
					for _, it := range items {
						fillColumns(it.(map[string]any), lead, root)
					}
					detail["read_lead"] = lead
					tree = sourceDetailTree(detail, false)
				}
			}
		}
		return api.ConnectorPanelOutput{DSLTree: tree, Data: map[string]any{"detail": detail}, Title: title}, nil
	}
	f, module, err := sourceFields(ctx, cwm, in.ID)
	if err != nil {
		return api.ConnectorPanelOutput{}, err
	}
	c, err := s.connector(ctx, f.Connector)
	if err != nil {
		return api.ConnectorPanelOutput{}, err
	}
	vbox := func(children ...any) map[string]any {
		return map[string]any{"kind": "vbox", "gap": "md", "children": children}
	}
	if in.Tab == api.SourceTabTake {
		selected, err := s.pickedRows(ctx, in.ID)
		if err != nil {
			return api.ConnectorPanelOutput{}, err
		}
		// A connector proposing from the holder's keywords opens on its
		// proposal region; any other keeps its picker.
		// A connector reading from another source's kept items opens on
		// them in groups under the link band (mock-up v6).
		if followsBrowse(c) {
			return s.fromTake(ctx, cwm, c, module, in.ID, title, selected)
		}
		if proposes(c) {
			return s.proposalTake(ctx, cwm, c, module, in.ID, title, selected)
		}
		// A connector picking in a check-box tree opens on it under its
		// head (« Pages prises », mock-up v6).
		if pickerView(c).Layout == "tree" {
			return s.treeTake(ctx, cwm, c, module, in.ID, title, selected), nil
		}
		return api.ConnectorPanelOutput{DSLTree: vbox(pickerNode(c, module)), Data: map[string]any{"selected": selected}, Title: title}, nil
	}
	// « Réglages » is how the source searches and reads (mock-up v6
	// vSourceSettings): its rows edited in place, then where
	// it reads - the connector's location form, never for a connector whose
	// picker selects.
	settings, value := s.settingsForm(ctx, cwm, c, f, in.ID)
	data := map[string]any{"settings": value}
	children := []any{settings}
	if form := locationForm(c, in.ID); form != nil {
		loc, err := locationData(ctx, cwm, in.ID)
		if err != nil {
			return api.ConnectorPanelOutput{}, err
		}
		data["location"] = loc
		children = append(children, form)
	}
	return api.ConnectorPanelOutput{DSLTree: vbox(children...), Data: data, Title: title}, nil
}

// unitPlural is a picker's unit in the plural, as a proposal bound counts it
// (« applications », « pages »); « éléments » when the connector names none.
func unitPlural(unit string) string {
	unit = strings.TrimSpace(unit)
	if unit == "" {
		return "éléments"
	}
	if strings.HasSuffix(unit, "s") || strings.HasSuffix(unit, "x") {
		return unit
	}
	return unit + "s"
}

// sourceHolder is the object a source belongs to - its parent - or "" when
// it cannot be read.
func sourceHolder(ctx context.Context, cwm cwmapi.CWMService, source string) string {
	module, err := cwmapi.ModuleOfObjectID(source)
	if err != nil {
		return ""
	}
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: source})
	if err != nil {
		return ""
	}
	return strings.TrimSpace(got.Object.ParentID())
}

// SourceTakeFunction is `source_take()` over an information source: the name
// of its « what it takes » tab, the take label its connector declares
// (mock-up v6) - « Ce qu'elle prend » when the connector
// declares none, is not contributed, or the object is no information source.
func (s *Service) SourceTakeFunction(ctx context.Context, call cwmapi.FunctionCall) (string, error) {
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return "", fmt.Errorf("sources: resolve CWMService: %w", err)
	}
	f, _, err := sourceFields(ctx, cwm, call.Object)
	if errors.Is(err, cwmapi.ErrInvalidInput) {
		return api.SourceTakeDefault, nil
	}
	if err != nil {
		return "", err
	}
	c, err := s.connector(ctx, f.Connector)
	if err != nil || strings.TrimSpace(c.Take) == "" {
		return api.SourceTakeDefault, nil
	}
	return strings.TrimSpace(c.Take), nil
}

// SourceSubTabsFunction is `source_sub_tabs()` over an information source:
// whether it opens on its three sub-tabs - its holder carries
// api.SubTabsCapability - or on the one « Source » tab. An
// object that is no information source, or a holder that cannot be read,
// answers false: the « Source » tab, as before the sub-tabs.
func (s *Service) SourceSubTabsFunction(ctx context.Context, call cwmapi.FunctionCall) (bool, error) {
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return false, fmt.Errorf("sources: resolve CWMService: %w", err)
	}
	module, err := cwmapi.ModuleOfObjectID(call.Object)
	if err != nil {
		return false, nil
	}
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: call.Object})
	if err != nil || got.Object.Type != api.InformationSourceType {
		return false, nil
	}
	holder := strings.TrimSpace(got.Object.ParentID())
	if holder == "" {
		return false, nil
	}
	ok, err := cwmapi.HasCapability(ctx, cwm, holder, api.SubTabsCapability)
	return err == nil && ok, nil
}
