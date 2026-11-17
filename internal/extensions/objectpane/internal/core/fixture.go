// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/objectpane/api"
)

// The fixture source: a panel's dummy data, read from a JSON file the
// panel's extension ships as a reference (`<fixture>.<state>.json`), so any
// application shows a screen with dummy data before its real source exists.
// The panel declares `source: ktw-agents:object-pane/fixture` and names the
// fixture in its `input`; swapping its `source:` for the real one later
// changes nothing else. Nothing here knows an application.

// DefaultFixtureState is the state read when none is asked and the object
// gives none.
const DefaultFixtureState = "default"

// fixtureState is a state a file name may carry: no separator, no `..`.
var fixtureState = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// fixtureReader reads one reference's bytes; found is false when the
// universe holds no such reference.
type fixtureReader func(ctx context.Context, uri string) (data []byte, found bool, err error)

// objectReader reads the object a state field is read from.
type objectReader func(ctx context.Context, id string) (cwmapi.Object, error)

// Fixture answers the fixture file for the state asked: the catalog's
// references read the file, CWM the object when the state is one of its
// fields.
func (s *Service) Fixture(ctx context.Context, in api.FixtureInput) (json.RawMessage, error) {
	return fixture(ctx, in, catalogFixtureReader, cwmObjectReader)
}

func fixture(ctx context.Context, in api.FixtureInput, read fixtureReader, object objectReader) (json.RawMessage, error) {
	base := strings.TrimSpace(in.Fixture)
	u, err := contribapi.ParseResourceURI(base + ".json")
	if base == "" || err != nil || u.Type != mbapi.ResourceTypeReference {
		return nil, fmt.Errorf("%w: fixture: %q is not a reference URI <provider>:/reference/<path>", cwmapi.ErrInvalidInput, in.Fixture)
	}
	state := strings.TrimSpace(in.State)
	if state == "" && in.StateField != "" && in.ID != "" {
		o, err := object(ctx, in.ID)
		if err != nil {
			return nil, err
		}
		state = stateAt(o, in.StateField)
	}
	if state == "" {
		state = DefaultFixtureState
	}
	if !fixtureState.MatchString(state) {
		return nil, nil // a state no file can carry: the panel's empty state
	}
	data, found, err := read(ctx, base+"."+state+".json")
	if err != nil || !found {
		return nil, err // an unknown state or a missing file: the empty state
	}
	if !json.Valid(data) {
		return nil, fmt.Errorf("fixture %s.%s.json is not JSON", base, state)
	}
	return json.RawMessage(data), nil
}

// stateAt is the object's value at a dotted path (`fields.mock`,
// `capabilities.<cap>.mock`) as text; "" when there is none.
func stateAt(o cwmapi.Object, path string) string {
	b, err := json.Marshal(o)
	if err != nil {
		return ""
	}
	var v any
	if json.Unmarshal(b, &v) != nil {
		return ""
	}
	for _, seg := range strings.Split(path, ".") {
		m, ok := v.(map[string]any)
		if !ok {
			return ""
		}
		v = m[seg]
	}
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

// catalogFixtureReader reads a reference of the universe: present among the
// catalog's references, then its file (the source layer) opened.
func catalogFixtureReader(ctx context.Context, uri string) ([]byte, bool, error) {
	cat, err := application.UseService[mbapi.Catalog](ctx)
	if err != nil || cat == nil {
		return nil, false, fmt.Errorf("object-pane: fixture: no catalog: %v", err)
	}
	q := mbapi.Q{Reach: mbapi.ReachUniverse, Kind: mbapi.ResourceTypeReference, Catalog: cat}
	refs, err := mbapi.Query(ctx, q)
	if err != nil {
		return nil, false, err
	}
	found := false
	for _, c := range refs {
		if c.ResolvedURI() == uri {
			found = true
			break
		}
	}
	if !found {
		return nil, false, nil
	}
	l, err := mbapi.Open(ctx, mbapi.Q{Reach: mbapi.ReachUniverse, Catalog: cat}, uri, contribapi.LayerSource)
	if err != nil {
		return nil, false, err
	}
	defer l.Close()
	b, err := l.Bytes()
	return b, err == nil, err
}

func cwmObjectReader(ctx context.Context, id string) (cwmapi.Object, error) {
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return cwmapi.Object{}, fmt.Errorf("object-pane: resolve CWMService: %w", err)
	}
	module, err := cwmapi.ModuleOfObjectID(id)
	if err != nil {
		return cwmapi.Object{}, err
	}
	got, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: id})
	if err != nil {
		return cwmapi.Object{}, err
	}
	return got.Object, nil
}
