// SPDX-License-Identifier: Apache-2.0

package authoring

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/signing"
)

// provenance_test.go — a packaged artifact carries a verifiable assertion.
//
// The chain is: sign at packaging → the assertions ship inside the pack → a
// consumer reads them, stamps them onto the contributions it derived, and
// checks them against a key it holds. Every step here is the real one; only the
// contribution list is supplied directly rather than probed, so the test needs
// no built binary.

func packContributions() []contribapi.Contribution {
	body := []byte(`{"name":"doc-create"}`)
	src := []byte("# doc-create\n\ninstructions")
	return []contribapi.Contribution{
		{
			Provider: "my-pack", Type: "skill", ID: "doc-create",
			Layers: map[string]contribapi.LayerRef{
				contribapi.LayerData:   contribapi.InlineData(body)[contribapi.LayerData],
				contribapi.LayerSource: {Hash: contribapi.HashContent(src), Size: int64(len(src))},
			},
		},
		{
			Provider: "my-pack", Type: "task", ID: "render",
			Layers: contribapi.InlineData([]byte(`{"name":"render"}`)),
		},
	}
}

func testKeys(t *testing.T) (signing.PublicKey, signing.SecretKey) {
	t.Helper()
	pk, sk, err := signing.Generate()
	if err != nil {
		t.Fatal(err)
	}
	return pk, sk
}

// TestPackaging_ProducesVerifiableAssertions is the headline: what packaging
// writes into a pack verifies, for every contribution, against the publisher's
// key.
func TestPackaging_ProducesVerifiableAssertions(t *testing.T) {
	folder := t.TempDir()
	pk, sk := testKeys(t)
	cs := packContributions()

	n, err := WriteAssertions(folder, cs, sk, "ACME Corp")
	if err != nil {
		t.Fatal(err)
	}
	if n != len(cs) {
		t.Fatalf("wrote %d assertions for %d contributions", n, len(cs))
	}

	// What ships inside the pack.
	as, err := ReadAssertions(folder)
	if err != nil {
		t.Fatal(err)
	}
	if len(as) != len(cs) {
		t.Fatalf("read back %d assertions, want %d", len(as), len(cs))
	}

	// A consumer derives the contributions from the folder and stamps the
	// claims onto them.
	stamped := StampAssertions(packContributions(), as)
	invalid, unsigned := signing.VerifyAll(stamped, pk)
	if len(invalid) != 0 || len(unsigned) != 0 {
		t.Fatalf("after packaging: invalid=%v unsigned=%v", invalid, unsigned)
	}
	for _, c := range stamped {
		if c.Provenance.Publisher != "ACME Corp" {
			t.Errorf("%s: publisher = %q", c.ResolvedURI(), c.Provenance.Publisher)
		}
	}
}

// TestPackaging_TamperedContentFailsVerification asserts the assertion is tied
// to content, not just to a name: swapping a layer's bytes after packaging
// breaks it, and the verifier never reads those bytes to find out.
func TestPackaging_TamperedContentFailsVerification(t *testing.T) {
	folder := t.TempDir()
	pk, sk := testKeys(t)
	if _, err := WriteAssertions(folder, packContributions(), sk, "ACME Corp"); err != nil {
		t.Fatal(err)
	}
	as, err := ReadAssertions(folder)
	if err != nil {
		t.Fatal(err)
	}

	tampered := packContributions()
	l := tampered[0].Layers[contribapi.LayerSource]
	l.Hash = contribapi.HashContent([]byte("# doc-create\n\nsomething else entirely"))
	tampered[0].Layers = map[string]contribapi.LayerRef{contribapi.LayerSource: l}

	invalid, _ := signing.VerifyAll(StampAssertions(tampered, as), pk)
	if len(invalid) != 1 || invalid[0] != "my-pack:/skill/doc-create" {
		t.Fatalf("tampering with a layer reported invalid=%v, want the skill", invalid)
	}
}

// TestPackaging_WithoutAKeyShipsUnsigned asserts publishing unsigned is a
// supported outcome, not a failure — and that an unsigned pack reads back as
// unsigned rather than as broken.
func TestPackaging_WithoutAKeyShipsUnsigned(t *testing.T) {
	folder := t.TempDir()
	n, err := SignFolder("", folder, "", "ACME Corp")
	if err != nil {
		t.Fatalf("packaging without a key failed: %v", err)
	}
	if n != 0 {
		t.Errorf("wrote %d assertions with no key", n)
	}
	if _, err := os.Stat(filepath.Join(folder, ProvenanceFile)); !os.IsNotExist(err) {
		t.Error("an unsigned pack carries a provenance file")
	}

	// And a consumer reading it gets an empty set, not an error.
	as, err := ReadAssertions(folder)
	if err != nil {
		t.Fatalf("reading an unsigned pack errored: %v", err)
	}
	if len(as) != 0 {
		t.Errorf("unsigned pack yielded %d assertions", len(as))
	}
	pk, _ := testKeys(t)
	stamped := StampAssertions(packContributions(), as)
	invalid, unsigned := signing.VerifyAll(stamped, pk)
	if len(invalid) != 0 {
		t.Errorf("unsigned contributions reported INVALID: %v", invalid)
	}
	if len(unsigned) != 2 {
		t.Errorf("unsigned = %v, want both contributions", unsigned)
	}
	for _, c := range stamped {
		if err := signing.VerifyContribution(c, pk); !errors.Is(err, signing.ErrNoProvenance) {
			t.Errorf("%s: err = %v, want ErrNoProvenance", c.ResolvedURI(), err)
		}
	}
}

// TestPackaging_IsDeterministic asserts an unchanged pack re-signed produces
// identical bytes. A packaging step whose output differed every run would make
// every rebuild look like a change — and the content digest that drives
// staleness would churn for no reason.
func TestPackaging_IsDeterministic(t *testing.T) {
	_, sk := testKeys(t)
	a, b := t.TempDir(), t.TempDir()
	if _, err := WriteAssertions(a, packContributions(), sk, "ACME Corp"); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteAssertions(b, packContributions(), sk, "ACME Corp"); err != nil {
		t.Fatal(err)
	}
	fa, err := os.ReadFile(filepath.Join(a, ProvenanceFile))
	if err != nil {
		t.Fatal(err)
	}
	fb, err := os.ReadFile(filepath.Join(b, ProvenanceFile))
	if err != nil {
		t.Fatal(err)
	}
	if string(fa) != string(fb) {
		t.Error("re-signing unchanged content produced different bytes")
	}
}
