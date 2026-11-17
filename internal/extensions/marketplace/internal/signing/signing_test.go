// SPDX-License-Identifier: Apache-2.0

package signing

import (
	"strings"
	"testing"
)

func TestSignVerify_RoundTrip(t *testing.T) {
	pk, sk, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	msg := []byte("apiVersion: 1\nmarketplace: {name: team}\nitems: []\n")
	sig := []byte(Sign(msg, sk, "team catalog v1"))

	if err := Verify(msg, sig, pk); err != nil {
		t.Fatalf("verify a good signature: %v", err)
	}
}

func TestVerify_TamperedMessageFails(t *testing.T) {
	pk, sk, _ := Generate()
	msg := []byte("trusted index bytes")
	sig := []byte(Sign(msg, sk, ""))

	if err := Verify([]byte("trusted index bytez"), sig, pk); err == nil {
		t.Fatal("expected verification to fail on a tampered message")
	}
}

func TestVerify_WrongKeyFails(t *testing.T) {
	_, sk, _ := Generate()
	other, _, _ := Generate() // a different keypair's public key
	msg := []byte("index")
	sig := []byte(Sign(msg, sk, ""))

	if err := Verify(msg, sig, other); err == nil {
		t.Fatal("expected verification to fail against the wrong public key")
	}
}

func TestPublicKey_RoundTripThroughFileAndLine(t *testing.T) {
	pk, _, _ := Generate()

	// full minisign file form
	file := EncodePublicKey(pk, "team key")
	got, err := ParsePublicKey([]byte(file))
	if err != nil || got.KeyID != pk.KeyID || string(got.Key) != string(pk.Key) {
		t.Fatalf("pubkey file round-trip: %v", err)
	}
	if !strings.HasPrefix(file, "untrusted comment:") {
		t.Fatal("public key file should carry a comment header")
	}

	// bare base64 line form (the stored subscription form)
	got2, err := ParsePublicKey([]byte(PublicKeyLine(pk)))
	if err != nil || got2.KeyID != pk.KeyID {
		t.Fatalf("pubkey line round-trip: %v", err)
	}
}

func TestSecretKey_RoundTrip(t *testing.T) {
	pk, sk, _ := Generate()
	got, err := ParseSecretKey([]byte(EncodeSecretKey(sk, "")))
	if err != nil || got.KeyID != sk.KeyID {
		t.Fatalf("secret key round-trip: %v", err)
	}
	// a signature from the round-tripped key still verifies against the pubkey
	sig := []byte(Sign([]byte("m"), got, ""))
	if err := Verify([]byte("m"), sig, pk); err != nil {
		t.Fatalf("round-tripped secret key must still sign verifiably: %v", err)
	}
}
