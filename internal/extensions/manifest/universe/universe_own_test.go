// SPDX-License-Identifier: Apache-2.0

package universe

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
)

// ownSkills is one binary's skill provider, in process.
type ownSkills struct {
	provider string
	layers   map[string]string // "<id>/<layer>" → content
}

func (p ownSkills) Type() string                                                 { return "skill" }
func (p ownSkills) Provider() string                                             { return p.provider }
func (p ownSkills) Identity() contribapi.Identity                                { return contribapi.Identity{Mutable: true} }
func (p ownSkills) Enumerate(context.Context) ([]contribapi.Contribution, error) { return nil, nil }
func (p ownSkills) Open(_ context.Context, id, layer string) (io.ReadCloser, contribapi.LayerRef, error) {
	body, ok := p.layers[id+"/"+layer]
	if !ok {
		return nil, contribapi.LayerRef{}, contribapi.ErrContributionNotFound{URI: p.provider + ":/skill/" + id}
	}
	return io.NopCloser(strings.NewReader(body)), contribapi.LayerRef{Size: int64(len(body))}, nil
}

// noUniverse is the aggregator's lane with no universe running.
var errNoUniverse = errors.New("universe catalog: no connection (universe unreachable)")

func noUniverseLayers(context.Context, string, string) ([]byte, contribapi.LayerRef, error) {
	return nil, contribapi.LayerRef{}, errNoUniverse
}

func noUniverseBodies(context.Context, string, string) ([]byte, error) { return nil, errNoUniverse }

// TestABinaryReadsItsOwnContributionsWithNoUniverse: a layer or a body of a
// contribution this binary publishes is read from its own providers, in
// process — with no hub, a binary prints its own skill
// another provider's still goes to the aggregator.
func TestABinaryReadsItsOwnContributionsWithNoUniverse(t *testing.T) {
	own := []contribapi.ContributionProvider{ownSkills{provider: "ktw-compose", layers: map[string]string{
		"test-artifact/instructions": "# Testing a katyoti artifact",
		"test-artifact/data":         "name: test-artifact",
	}}}
	ctx := context.Background()

	layers := OwnFirstLayers("ktw-compose", own, noUniverseLayers)
	body, _, err := layers(ctx, "ktw-compose:/skill/test-artifact", "instructions")
	if err != nil || string(body) != "# Testing a katyoti artifact" {
		t.Fatalf("own layer = %q, %v; want the skill's instructions, in process", body, err)
	}
	bodies := OwnFirstBodies("ktw-compose", own, noUniverseBodies)
	if b, err := bodies(ctx, "sha256:x", "ktw-compose:/skill/test-artifact"); err != nil || string(b) != "name: test-artifact" {
		t.Fatalf("own body = %q, %v; want its data layer, in process", b, err)
	}

	// Another provider's goes to the aggregator — here, unreachable.
	if _, _, err := layers(ctx, "ktw-agents:/skill/work-loop", "instructions"); !errors.Is(err, errNoUniverse) {
		t.Fatalf("another provider's layer: err = %v, want the aggregator's", err)
	}
	// An id this binary does not publish is not found, never asked elsewhere.
	if _, _, err := layers(ctx, "ktw-compose:/skill/none", "instructions"); err == nil || errors.Is(err, errNoUniverse) {
		t.Fatalf("an unknown own id: err = %v, want not found, in process", err)
	}
}
