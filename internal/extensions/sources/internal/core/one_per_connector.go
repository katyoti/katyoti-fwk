// SPDX-License-Identifier: Apache-2.0

package core

// ONE SOURCE PER CONNECTOR, FOR THE HOLDERS THAT ASK FOR IT:
// a holder whose type composes the capability
// one-source-per-connector (api.OnePerConnectorCapability - the as-is study
// v2 opts in) reads a connector through one source. Sources owns
// information-source, so it holds the rule in its before_create gate
// (SourceCheck): a source about to be created under such a holder that
// already has an active source of its connector is refused, « <source> est
// déjà dans l'étude » (« dans le module » under a module that opts in). A
// holder without the capability - a module, an as-is study v1 - is never
// refused. An archived source (« Retirer ») has left its holder, so its
// connector is free again; a source written again - an update, the same id -
// is never refused, so a holder that kept two of a connector before the rule
// still saves them.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/sources/api"
)

// moduleType is the type of a plain module holder: its refusal says « dans
// le module »; any other holder is a study's « dans l'étude ».
const moduleType = "module"

// heldConnector answers the reason a source about to be created is refused -
// its holder already reads its connector through another active source - or
// "" when it is accepted.
func (s *Service) heldConnector(ctx context.Context, src cwmapi.Object) string {
	connector, _ := src.Fields["connector"].(string)
	connector = strings.TrimSpace(connector)
	holder := strings.TrimSpace(src.ParentID())
	if connector == "" || holder == "" {
		return ""
	}
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return ""
	}
	if !onePerConnector(ctx, cwm, holder) {
		return ""
	}
	module, err := cwmapi.ModuleOfObjectID(holder)
	if err != nil {
		return ""
	}
	if src.ID != "" {
		if _, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: src.ID}); err == nil {
			return "" // written again: the rule gates a creation only
		}
	}
	list, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Module: module, Parent: holder, Type: api.InformationSourceType})
	if err != nil {
		return ""
	}
	for _, o := range list.Objects {
		if o.ID == src.ID {
			continue
		}
		// A list answers what is resident; the connector is read whole.
		if got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: o.ID}); err == nil {
			o = got.Object
		}
		var f api.InformationSourceFields
		_ = json.Unmarshal(o.FieldsJSON(), &f)
		if strings.TrimSpace(f.Connector) != connector {
			continue
		}
		name := strings.TrimSpace(o.Name)
		if name == "" {
			if c, err := s.connector(ctx, connector); err == nil && c.Label != "" {
				name = c.Label
			} else {
				name = connector
			}
		}
		where := "l'étude"
		if h, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: holder}); err == nil && h.Object.Type == moduleType {
			where = "le module"
		}
		return fmt.Sprintf("%s est déjà dans %s : une seule source par connecteur.", name, where)
	}
	return ""
}

// onePerConnector reports whether the holder asks for one source per
// connector: it carries api.OnePerConnectorCapability (its type's bundle, its
// aspects or its own choice). An unreadable holder asks for nothing.
func onePerConnector(ctx context.Context, cwm cwmapi.CWMService, holder string) bool {
	ok, err := cwmapi.HasCapability(ctx, cwm, holder, api.OnePerConnectorCapability)
	return err == nil && ok
}
