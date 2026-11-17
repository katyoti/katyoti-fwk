// SPDX-License-Identifier: Apache-2.0

// Package passkeytest is a software authenticator for tests: a phone whose
// passkey answers WebAuthn create and get as a real one would.
package passkeytest

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"testing"

	"github.com/go-webauthn/webauthn/protocol/webauthncbor"
	"github.com/go-webauthn/webauthn/protocol/webauthncose"
)

var b64 = base64.RawURLEncoding

// Phone is a software authenticator: a P-256 key, a credential id and a sign
// counter, answering create and get as a phone's would.
type Phone struct {
	key   *ecdsa.PrivateKey
	id    []byte
	Count uint32
	UV    bool // user verification: Face ID passed
	rpID  string
}

func NewPhone(t testing.TB, rpID string) *Phone {
	t.Helper()
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	id := make([]byte, 16)
	_, _ = rand.Read(id)
	return &Phone{key: k, id: id, UV: true, rpID: rpID}
}

func (p *Phone) authData(attested bool) []byte {
	h := sha256.Sum256([]byte(p.rpID))
	flags := byte(0x01) // user present
	if p.UV {
		flags |= 0x04
	}
	if attested {
		flags |= 0x40
	}
	out := append(h[:], flags)
	var c [4]byte
	binary.BigEndian.PutUint32(c[:], p.Count)
	out = append(out, c[:]...)
	if attested {
		out = append(out, make([]byte, 16)...) // aaguid
		var l [2]byte
		binary.BigEndian.PutUint16(l[:], uint16(len(p.id)))
		out = append(out, l[:]...)
		out = append(out, p.id...)
		cose, _ := webauthncbor.Marshal(webauthncose.EC2PublicKeyData{
			PublicKeyData: webauthncose.PublicKeyData{KeyType: 2, Algorithm: -7},
			Curve:         1, XCoord: p.key.X.FillBytes(make([]byte, 32)), YCoord: p.key.Y.FillBytes(make([]byte, 32)),
		})
		out = append(out, cose...)
	}
	return out
}

func challengeOf(t testing.TB, options json.RawMessage) string {
	t.Helper()
	var o struct {
		PublicKey struct {
			Challenge string `json:"challenge"`
		} `json:"publicKey"`
	}
	if err := json.Unmarshal(options, &o); err != nil || o.PublicKey.Challenge == "" {
		t.Fatalf("options %s: %v", options, err)
	}
	return o.PublicKey.Challenge
}

func clientData(kind, challenge, origin string) []byte {
	b, _ := json.Marshal(map[string]string{"type": kind, "challenge": challenge, "origin": origin})
	return b
}

// Create answers navigator.credentials.create at origin.
func (p *Phone) Create(t testing.TB, origin string, options json.RawMessage) []byte {
	t.Helper()
	att, _ := webauthncbor.Marshal(map[string]any{"fmt": "none", "attStmt": map[string]any{}, "authData": p.authData(true)})
	body, _ := json.Marshal(map[string]any{"id": b64.EncodeToString(p.id), "rawId": b64.EncodeToString(p.id), "type": "public-key",
		"response": map[string]any{"attestationObject": b64.EncodeToString(att),
			"clientDataJSON": b64.EncodeToString(clientData("webauthn.create", challengeOf(t, options), origin))}})
	return body
}

// Get answers navigator.credentials.get at origin, signing with the phone's key.
func (p *Phone) Get(t testing.TB, origin string, options json.RawMessage) []byte {
	t.Helper()
	p.Count++
	ad := p.authData(false)
	cd := clientData("webauthn.get", challengeOf(t, options), origin)
	h := sha256.Sum256(cd)
	digest := sha256.Sum256(append(append([]byte{}, ad...), h[:]...)) // ES256 signs the SHA-256 of authData || hash(clientData)
	sig, err := ecdsa.SignASN1(rand.Reader, p.key, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{"id": b64.EncodeToString(p.id), "rawId": b64.EncodeToString(p.id), "type": "public-key",
		"response": map[string]any{"authenticatorData": b64.EncodeToString(ad), "clientDataJSON": b64.EncodeToString(cd),
			"signature": b64.EncodeToString(sig), "userHandle": b64.EncodeToString([]byte("jdoe"))}})
	return body
}
