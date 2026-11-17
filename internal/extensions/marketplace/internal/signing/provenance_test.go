// SPDX-License-Identifier: Apache-2.0

package signing

import (
	"encoding/json"
	"errors"
	"testing"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
)

// provenance_test.go — origin is expressible and checkable.
//
// What this module owes is exactly that: a publisher can assert where a
// contribution came from, and a consumer can check the assertion. What a
// consumer DOES about the answer is policy, and deliberately not tested here
// because it is deliberately not implemented here.

func signedSkill(t *testing.T) contribapi.Contribution {
	t.Helper()
	body := []byte(`{"name":"doc-create"}`)
	source := []byte("# doc-create\n\nthe instructions")
	return contribapi.Contribution{
		Provider: "ktw-docs", Type: "skill", ID: "doc-create",
		Layers: map[string]contribapi.LayerRef{
			contribapi.LayerData: contribapi.InlineData(body)[contribapi.LayerData],
			contribapi.LayerSource: {
				Hash: contribapi.HashContent(source), Size: int64(len(source)),
				MediaType: "text/markdown",
			},
		},
	}
}

func keys(t *testing.T) (PublicKey, SecretKey) {
	t.Helper()
	pk, sk, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	return pk, sk
}

// TestProvenance_RoundTrips asserts a signed contribution verifies against the
// key that signed it.
func TestProvenance_RoundTrips(t *testing.T) {
	pk, sk := keys(t)
	c := signedSkill(t)
	c.Provenance = ptr(SignContribution(c, sk, "ACME Corp"))

	if !c.Signed() {
		t.Fatal("a signed contribution does not report itself signed")
	}
	if err := VerifyContribution(c, pk); err != nil {
		t.Fatalf("verify: %v", err)
	}
	if c.Provenance.Publisher != "ACME Corp" || c.Provenance.KeyID == "" {
		t.Errorf("assertion = %+v", c.Provenance)
	}
}

// TestProvenance_TamperingWithAnyLayerInvalidates is the property the whole
// mechanism exists for: identity covers every hash, so changing content changes
// identity, and the assertion no longer holds — WITHOUT the verifier reading
// the content.
func TestProvenance_TamperingWithAnyLayerInvalidates(t *testing.T) {
	pk, sk := keys(t)
	original := signedSkill(t)
	assertion := SignContribution(original, sk, "ACME Corp")

	for name, tamper := range map[string]func(contribapi.Contribution) contribapi.Contribution{
		"data layer content": func(c contribapi.Contribution) contribapi.Contribution {
			c.SetData([]byte(`{"name":"doc-create-EVIL"}`))
			return c
		},
		"source layer content": func(c contribapi.Contribution) contribapi.Contribution {
			l := c.Layers[contribapi.LayerSource]
			l.Hash = contribapi.HashContent([]byte("# doc-create\n\nrun this instead"))
			c.Layers = map[string]contribapi.LayerRef{contribapi.LayerSource: l}
			return c
		},
		"source layer size": func(c contribapi.Contribution) contribapi.Contribution {
			l := c.Layers[contribapi.LayerSource]
			l.Size = 999999
			c.Layers = map[string]contribapi.LayerRef{contribapi.LayerSource: l}
			return c
		},
		"re-pointed URI": func(c contribapi.Contribution) contribapi.Contribution {
			c.ID = "doc-create-v2"
			return c
		},
		"changed kind": func(c contribapi.Contribution) contribapi.Contribution {
			c.Type = "script"
			return c
		},
		"added layer": func(c contribapi.Contribution) contribapi.Contribution {
			c.Layers = map[string]contribapi.LayerRef{
				contribapi.LayerSource: c.Layers[contribapi.LayerSource],
				"payload":              {Hash: "sha256:deadbeef", Size: 7},
			}
			return c
		},
	} {
		t.Run(name, func(t *testing.T) {
			tampered := tamper(signedSkill(t))
			tampered.Provenance = ptr(assertion) // the ORIGINAL assertion, carried along
			err := VerifyContribution(tampered, pk)
			if err == nil {
				t.Fatalf("tampering with %s left the assertion valid", name)
			}
			if errors.Is(err, ErrNoProvenance) {
				t.Fatalf("tampering with %s reported as unsigned rather than invalid", name)
			}
		})
	}
}

// TestProvenance_UnsignedIsValidAndDistinguishable asserts an unsigned
// contribution is usable, and that "no assertion" is a different answer from
// "the assertion failed" — a consumer applying a policy has to tell them apart.
func TestProvenance_UnsignedIsValidAndDistinguishable(t *testing.T) {
	pk, _ := keys(t)
	c := signedSkill(t)

	if c.Signed() {
		t.Error("an unsigned contribution reports itself signed")
	}
	err := VerifyContribution(c, pk)
	if !errors.Is(err, ErrNoProvenance) {
		t.Fatalf("unsigned: err = %v, want ErrNoProvenance", err)
	}
	// And it serialises without a provenance key at all — absent, not empty.
	b, mErr := json.Marshal(c)
	if mErr != nil {
		t.Fatal(mErr)
	}
	if bytesContain(b, "provenance") {
		t.Errorf("an unsigned contribution carries a provenance key: %s", b)
	}
}

// TestProvenance_WrongKeyFails asserts the assertion is tied to a key, not just
// to well-formedness.
func TestProvenance_WrongKeyFails(t *testing.T) {
	_, sk := keys(t)
	otherPub, _ := keys(t)
	c := signedSkill(t)
	c.Provenance = ptr(SignContribution(c, sk, "ACME Corp"))

	if err := VerifyContribution(c, otherPub); err == nil {
		t.Fatal("an assertion verified against a key that did not sign it")
	}
}

// TestIdentityDigest_IsCanonical asserts two producers of the same contribution
// compute the same digest, and that no rearrangement of field values collides.
func TestIdentityDigest_IsCanonical(t *testing.T) {
	a := signedSkill(t)
	b := signedSkill(t)
	// Same content, layers inserted in a different order.
	b.Layers = map[string]contribapi.LayerRef{}
	for k, v := range a.Layers {
		b.Layers[k] = v
	}
	b.Layers["zzz"] = contribapi.LayerRef{Hash: "sha256:aa", Size: 1}
	a.Layers["zzz"] = contribapi.LayerRef{Hash: "sha256:aa", Size: 1}
	if a.IdentityDigest() != b.IdentityDigest() {
		t.Error("the same contribution produced two digests — map order leaked into the result")
	}

	// Length-prefixing: a boundary shift must not produce the same digest.
	x := contribapi.Contribution{Provider: "p", Type: "ab", ID: "c"}
	y := contribapi.Contribution{Provider: "p", Type: "a", ID: "bc"}
	if x.IdentityDigest() == y.IdentityDigest() {
		t.Error("different (kind, id) splits hashed alike — fields are not delimited")
	}
}

// TestExecutionOf_DefaultsToDeclarative asserts the safe default: a kind that
// does not say is READ, never RUN. An unknown kind becoming executable by
// accident is the failure worth preventing.
func TestExecutionOf_DefaultsToDeclarative(t *testing.T) {
	for _, in := range []string{"", "  ", "nonsense", "Declarative", "declarative"} {
		if got := contribapi.ExecutionOf(in); got != contribapi.ExecutionDeclarative {
			t.Errorf("ExecutionOf(%q) = %q, want declarative", in, got)
		}
	}
	for _, in := range []string{"executable", "Executable", " EXECUTABLE "} {
		if got := contribapi.ExecutionOf(in); got != contribapi.ExecutionExecutable {
			t.Errorf("ExecutionOf(%q) = %q, want executable", in, got)
		}
	}
}

func ptr(p contribapi.Provenance) *contribapi.Provenance { return &p }

func bytesContain(b []byte, s string) bool {
	return len(b) > 0 && len(s) > 0 && indexOf(string(b), s) >= 0
}

func indexOf(h, n string) int {
	for i := 0; i+len(n) <= len(h); i++ {
		if h[i:i+len(n)] == n {
			return i
		}
	}
	return -1
}
