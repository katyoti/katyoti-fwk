// SPDX-License-Identifier: Apache-2.0

package refmatch

import "testing"

func items() []Item {
	return []Item{
		{Provider: "ktw-agents", Name: "cwm-create", Advertised: true},
		{Provider: "ktw-agents", Name: "cwm-verify", Advertised: true},
		{Provider: "ktw-agents", Name: "cwm-internal", Advertised: false},
		{Provider: "ktw-weather", Name: "get-forecast", Advertised: true},
		{Provider: "ktw-weather", Name: "secret-debug", Advertised: false},
	}
}

func names(items []Item) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.Provider + "/" + it.Name
	}
	return out
}

func eq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestMatch_StarIsAllAdvertised(t *testing.T) {
	got := names(Match(items(), []Selector{{Name: "*"}}))
	want := []string{"ktw-agents/cwm-create", "ktw-agents/cwm-verify", "ktw-weather/get-forecast"}
	if !eq(got, want) {
		t.Errorf("`*` = %v, want all advertised %v (unadvertised must NOT leak)", got, want)
	}
}

func TestMatch_BareNameIsAllAdvertised(t *testing.T) {
	got := names(Match(items(), []Selector{{}}))
	if len(got) != 3 {
		t.Errorf("empty selector = %v, want the 3 advertised", got)
	}
}

func TestMatch_ExactNameReachesUnadvertised(t *testing.T) {
	got := names(Match(items(), []Selector{{Name: "cwm-internal"}}))
	if !eq(got, []string{"ktw-agents/cwm-internal"}) {
		t.Errorf("exact name = %v, want the unadvertised item (explicit opt-in)", got)
	}
}

func TestMatch_GlobIsAdvertisedOnly(t *testing.T) {
	// cwm-* must NOT pull in cwm-internal (unadvertised) — a glob is bulk.
	got := names(Match(items(), []Selector{{Name: "cwm-*"}}))
	if !eq(got, []string{"ktw-agents/cwm-create", "ktw-agents/cwm-verify"}) {
		t.Errorf("cwm-* = %v, want only advertised cwm-* (no cwm-internal)", got)
	}
}

func TestMatch_ProviderScoped(t *testing.T) {
	got := names(Match(items(), []Selector{{Provider: "ktw-weather", Name: "*"}}))
	if !eq(got, []string{"ktw-weather/get-forecast"}) {
		t.Errorf("ktw-weather/* = %v, want only the weather advertised tool", got)
	}
}

func TestMatch_ProviderScopedExactReachesUnadvertised(t *testing.T) {
	got := names(Match(items(), []Selector{{Provider: "ktw-weather", Name: "secret-debug"}}))
	if !eq(got, []string{"ktw-weather/secret-debug"}) {
		t.Errorf("provider-scoped exact = %v, want the unadvertised secret-debug", got)
	}
}

func TestMatch_UnionDedup(t *testing.T) {
	// two selectors that overlap on cwm-create → one result each, deduped.
	got := names(Match(items(), []Selector{{Name: "cwm-*"}, {Name: "cwm-create"}}))
	if !eq(got, []string{"ktw-agents/cwm-create", "ktw-agents/cwm-verify"}) {
		t.Errorf("union = %v, want deduped advertised cwm-* set", got)
	}
}

func TestMatch_EmptySelectorsNil(t *testing.T) {
	if got := Match(items(), nil); got != nil {
		t.Errorf("no selectors = %v, want nil", got)
	}
}
