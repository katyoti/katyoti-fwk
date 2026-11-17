// SPDX-License-Identifier: Apache-2.0

package contribapi_test

import (
	"encoding/json"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
)

func TestContribution_resolvedURI(t *testing.T) {
	// URI field unset → derived from Provider/Type/ID.
	c := contribapi.Contribution{
		Provider: "ktw-docs",
		Type:     "skill",
		ID:       "doc-create",
	}
	if got, want := c.ResolvedURI(), "ktw-docs:/skill/doc-create"; got != want {
		t.Errorf("ResolvedURI = %q, want %q", got, want)
	}

	// URI field set → echoed.
	c.URI = "ktw-docs:/skill/doc-create"
	if got, want := c.ResolvedURI(), "ktw-docs:/skill/doc-create"; got != want {
		t.Errorf("ResolvedURI (explicit) = %q, want %q", got, want)
	}
}

func TestContribution_resolvedSchemaVersion(t *testing.T) {
}

func TestContributionMeta_isAdvertised(t *testing.T) {
	// nil → default true.
	if got := (contribapi.ContributionMeta{}).IsAdvertised(); !got {
		t.Errorf("IsAdvertised(nil) = false, want true (default)")
	}
	yes, no := true, false
	if got := (contribapi.ContributionMeta{Advertised: &yes}).IsAdvertised(); !got {
		t.Errorf("IsAdvertised(*true) = false, want true")
	}
	if got := (contribapi.ContributionMeta{Advertised: &no}).IsAdvertised(); got {
		t.Errorf("IsAdvertised(*false) = true, want false")
	}
}

// TestContribution_jsonRoundTrip asserts canonical JSON stability: a
// Contribution marshalled then unmarshalled compares byte-equal.
// Important for content-addressed hashing of the manifest envelope.
func TestContribution_jsonRoundTrip(t *testing.T) {
	yes := true
	original := contribapi.Contribution{
		Provider: "ktw-docs",
		Type:     "skill",
		ID:       "doc-create",
		URI:      "ktw-docs:/skill/doc-create",
		Metadata: &contribapi.ContributionMeta{
			Name:        "Doc Create",
			Description: "Create documents in various formats",
			Icon:        &contribapi.ContributionIcon{Tabler: "file-text", Emoji: "📄"},
			Hashtags:    []string{"documents", "writing"},
			Category:    "writing",
			Advertised:  &yes,
			Annotations: map[string]string{"vendor": "ktw"},
		},
		Layers: contribapi.InlineData(json.RawMessage(`{"instructions":"step 1\nstep 2","requires":["workspace"]}`)),
	}

	bs, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var round contribapi.Contribution
	if err := json.Unmarshal(bs, &round); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	bs2, err := json.Marshal(round)
	if err != nil {
		t.Fatalf("re-marshal: %v", err)
	}

	if string(bs) != string(bs2) {
		t.Errorf("round-trip not byte-stable\nfirst:  %s\nsecond: %s", bs, bs2)
	}
}

// TestContribution_jsonOmitOptional asserts that the empty
// Contribution emits a minimal JSON document — only the three required
// fields, no junk from default-zero values.
func TestContribution_jsonOmitOptional(t *testing.T) {
	c := contribapi.Contribution{
		Provider: "p",
		Type:     "t",
		ID:       "i",
	}
	bs, err := json.Marshal(c)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	want := `{"provider":"p","type":"t","id":"i"}`
	if string(bs) != want {
		t.Errorf("minimal marshal = %s, want %s", bs, want)
	}
}
