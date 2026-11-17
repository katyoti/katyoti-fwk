// SPDX-License-Identifier: Apache-2.0

package resolver

import (
	"reflect"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

func brief(id, topic string) api.ObjectBrief {
	b := api.ObjectBrief{ID: id, Type: "rule"}
	if topic != "" {
		b.StructuredFields = map[string]string{"topic": topic}
	}
	return b
}

func ids(bs []api.ObjectBrief) []string {
	out := []string{}
	for _, b := range bs {
		out = append(out, b.ID)
	}
	return out
}

type strategyCase struct {
	name                      string
	in                        StrategyInput
	want, wantLocal, wantTree []string
	wantType                  []string
}

func runStrategyCases(t *testing.T, strategy string, cases []strategyCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, bs, ok := Reduce(strategy, tc.in)
			if !ok {
				t.Fatalf("Reduce(%q) unknown", strategy)
			}
			for _, c := range []struct {
				what      string
				got, want []string
			}{
				{"items", ids(got), tc.want},
				{"by-source local", ids(bs.Local), tc.wantLocal},
				{"by-source tree", ids(bs.Tree), tc.wantTree},
				{"by-source type", ids(bs.Type), tc.wantType},
			} {
				want := c.want
				if want == nil {
					want = []string{}
				}
				if !reflect.DeepEqual(c.got, want) {
					t.Errorf("%s = %v, want %v", c.what, c.got, want)
				}
			}
		})
	}
}

func TestReduce_AccumulateWithLocalOverride(t *testing.T) {
	runStrategyCases(t, "accumulate-with-local-override", []strategyCase{
		{
			name: "local shadows tree and type on the same key",
			in: StrategyInput{
				OverrideKey: "topic",
				Local:       []api.ObjectBrief{brief("l1", "naming")},
				Tree:        []api.ObjectBrief{brief("t1", "naming"), brief("t2", "errors")},
				Types:       []api.ObjectBrief{brief("y1", "naming"), brief("y2", "tests")},
			},
			want:      []string{"l1", "t2", "y2"},
			wantLocal: []string{"l1"}, wantTree: []string{"t2"}, wantType: []string{"y2"},
		},
		{
			name: "same key between tree and type is NOT shadowed (only local overrides)",
			in: StrategyInput{
				OverrideKey: "topic",
				Tree:        []api.ObjectBrief{brief("t1", "naming"), brief("t2", "naming")},
				Types:       []api.ObjectBrief{brief("y1", "naming")},
			},
			want:     []string{"t1", "t2", "y1"},
			wantTree: []string{"t1", "t2"}, wantType: []string{"y1"},
		},
		{
			name: "keyless briefs are never shadowed and keyless local shadows nothing",
			in: StrategyInput{
				OverrideKey: "topic",
				Local:       []api.ObjectBrief{brief("l1", "")},
				Tree:        []api.ObjectBrief{brief("t1", "")},
				Types:       []api.ObjectBrief{brief("y1", "")},
			},
			want:      []string{"l1", "t1", "y1"},
			wantLocal: []string{"l1"}, wantTree: []string{"t1"}, wantType: []string{"y1"},
		},
		{
			name: "all local items kept even when two share a key",
			in: StrategyInput{
				OverrideKey: "topic",
				Local:       []api.ObjectBrief{brief("l1", "naming"), brief("l2", "naming")},
				Tree:        []api.ObjectBrief{brief("t1", "naming")},
			},
			want:      []string{"l1", "l2"},
			wantLocal: []string{"l1", "l2"},
		},
		{
			name: "without an override key degrades to accumulate (dedupe by id)",
			in: StrategyInput{
				Local: []api.ObjectBrief{brief("a", "naming")},
				Tree:  []api.ObjectBrief{brief("a", "naming"), brief("t1", "naming")},
				Types: []api.ObjectBrief{brief("y1", "naming")},
			},
			want:      []string{"a", "t1", "y1"},
			wantLocal: []string{"a"}, wantTree: []string{"t1"}, wantType: []string{"y1"},
		},
		{
			name: "empty input gives nothing",
			in:   StrategyInput{OverrideKey: "topic"},
		},
	})
}

func TestReduce_LocalOnly(t *testing.T) {
	runStrategyCases(t, "local-only", []strategyCase{
		{
			name: "tree and type are ignored",
			in: StrategyInput{
				OverrideKey: "topic",
				Local:       []api.ObjectBrief{brief("l1", "naming"), brief("l2", "naming")},
				Tree:        []api.ObjectBrief{brief("t1", "errors")},
				Types:       []api.ObjectBrief{brief("y1", "tests")},
			},
			want:      []string{"l1", "l2"},
			wantLocal: []string{"l1", "l2"},
		},
		{
			name: "no local means nothing even with tree and type",
			in: StrategyInput{
				Tree:  []api.ObjectBrief{brief("t1", "")},
				Types: []api.ObjectBrief{brief("y1", "")},
			},
		},
		{
			name: "local order kept, no dedupe",
			in: StrategyInput{
				Local: []api.ObjectBrief{brief("b", ""), brief("a", ""), brief("b", "")},
			},
			want:      []string{"b", "a", "b"},
			wantLocal: []string{"b", "a", "b"},
		},
	})
}

func TestReduce_UnknownStrategy(t *testing.T) {
	if _, _, ok := Reduce("nope", StrategyInput{}); ok {
		t.Fatal("unknown strategy reported as known")
	}
	want := []string{"accumulate", "accumulate-with-local-override", "closest-wins", "local-only"}
	if got := StrategyNames(); !reflect.DeepEqual(got, want) {
		t.Errorf("StrategyNames = %v, want %v", got, want)
	}
}
