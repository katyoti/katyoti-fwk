// SPDX-License-Identifier: Apache-2.0

package census

import (
	"context"
	"io"
	"strings"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	manifestlocal "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/local"
)

// The identity guard can fail: an immutable provider that returns an empty
// digest is refused, and so is a mutable one claiming a digest (e25, N3/N5 —
// the guard it replaces could not fail, because the builder wrote the flag it
// checked unconditionally).
func TestIdentityGuard_ImmutableNeedsADigestMutableHasNone(t *testing.T) {
	ok := &mbapi.Manifest{Providers: []mbapi.ProviderInfo{
		{Name: "ktw-w", Type: "skill", Digest: "sha256:abc"},
		{Name: "ktw-w", Type: "task", Mutable: true},
	}}
	if bad := identityRuleViolations(ok); len(bad) != 0 {
		t.Fatalf("a correct manifest was refused: %v", bad)
	}

	broken := &mbapi.Manifest{Providers: []mbapi.ProviderInfo{
		{Name: "ktw-w", Type: "skill"},                                   // immutable, no digest
		{Name: "ktw-w", Type: "task", Mutable: true, Digest: "sha256:x"}, // mutable, a digest
	}}
	bad := identityRuleViolations(broken)
	joined := strings.Join(bad, "\n")
	if len(bad) != 2 || !strings.Contains(joined, "ktw-w/skill: immutable provider carries no digest") ||
		!strings.Contains(joined, "ktw-w/task: mutable provider carries a digest") {
		t.Fatalf("findings = %v, want both rows named", bad)
	}
}

// Falsified on the path the goldens take: a provider whose Identity returns an
// empty digest while claiming immutability goes through the real builder, and
// the row it leaves is refused.
func TestIdentityGuard_RefusesAnImmutableProviderWithoutADigest(t *testing.T) {
	m, err := manifestlocal.NewBuilder("ktw-w", "dev").BuildFromProviders(context.Background(),
		[]contribapi.ContributionProvider{silentProvider{}})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	bad := identityRuleViolations(m)
	if len(bad) != 1 || !strings.Contains(bad[0], "ktw-w/system-less: immutable provider carries no digest") {
		t.Fatalf("findings = %v, want the silent provider refused", bad)
	}
}

// silentProvider publishes nothing and claims an immutable source without
// naming its version: the defect the guard exists for.
type silentProvider struct{}

func (silentProvider) Type() string                  { return "system-less" }
func (silentProvider) Provider() string              { return "ktw-w" }
func (silentProvider) Identity() contribapi.Identity { return contribapi.Identity{} }
func (silentProvider) Enumerate(context.Context) ([]contribapi.Contribution, error) {
	return nil, nil
}
func (p silentProvider) Open(ctx context.Context, id, layer string) (io.ReadCloser, contribapi.LayerRef, error) {
	return contribapi.OpenEnumerated(ctx, p, id, layer)
}
