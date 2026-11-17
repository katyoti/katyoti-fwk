// SPDX-License-Identifier: Apache-2.0

// Package signing is the neutral Ed25519 / minisign signing library shared by
// the katyoti CONSUMER (verify a catalog's provenance before install) and the
// ktw-compose PUBLISHER (sign a catalog index). It implements the scheme from
// modules/katyoti/team-marketplace.md: sign the index, hash-pin the
// assets.
//
// Verification is minisign-compatible (a signature produced by the `minisign`
// CLI verifies here, legacy OR prehashed mode) and uses only crypto/ed25519 +
// x/crypto/blake2b — no external `minisign` binary at verify time.
//
// The PUBLIC KEY + SIGNATURE files are minisign-format. The SECRET KEY file is
// katyoti's own (unencrypted) format — a convenience for the ktw-compose
// signer; the signature it produces is still minisign-verifiable. (Password-
// protecting the secret key, like minisign's own .key, is a follow-up.)
package signing

import (
	"bufio"
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/blake2b"
)

// Signature algorithm tags (the first 2 bytes of a minisign key/sig blob).
const (
	algLegacy    = "Ed" // the ed25519 signature is over the raw message
	algPrehashed = "ED" // the ed25519 signature is over blake2b-512(message)
)

// PublicKey is a parsed minisign public key: an 8-byte key id + ed25519 key.
type PublicKey struct {
	KeyID [8]byte
	Key   ed25519.PublicKey
}

// SecretKey is a parsed (unencrypted) signing key — katyoti's own format.
type SecretKey struct {
	KeyID [8]byte
	Key   ed25519.PrivateKey
}

// Generate creates a fresh keypair with a random key id.
func Generate() (PublicKey, SecretKey, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return PublicKey{}, SecretKey{}, err
	}
	var id [8]byte
	if _, err := rand.Read(id[:]); err != nil {
		return PublicKey{}, SecretKey{}, err
	}
	return PublicKey{KeyID: id, Key: pub}, SecretKey{KeyID: id, Key: priv}, nil
}

// PublicKeyLine returns the bare base64 key blob (no comment) — the compact
// form stored in a subscription's pinned-key field.
func PublicKeyLine(pk PublicKey) string {
	blob := make([]byte, 0, 2+8+ed25519.PublicKeySize)
	blob = append(blob, algLegacy...)
	blob = append(blob, pk.KeyID[:]...)
	blob = append(blob, pk.Key...)
	return base64.StdEncoding.EncodeToString(blob)
}

// EncodePublicKey renders a minisign-compatible public key file.
func EncodePublicKey(pk PublicKey, comment string) string {
	if comment == "" {
		comment = "katyoti public key"
	}
	return "untrusted comment: " + comment + "\n" + PublicKeyLine(pk) + "\n"
}

// ParsePublicKey parses a minisign public key file (2-line) OR a bare base64
// key line (the stored subscription form).
func ParsePublicKey(data []byte) (PublicKey, error) {
	lines := payloadLines(data)
	if len(lines) == 0 {
		return PublicKey{}, fmt.Errorf("public key: no key line")
	}
	raw, err := base64.StdEncoding.DecodeString(lines[0])
	if err != nil {
		return PublicKey{}, fmt.Errorf("public key: base64: %w", err)
	}
	if len(raw) != 2+8+ed25519.PublicKeySize {
		return PublicKey{}, fmt.Errorf("public key: want %d bytes, got %d", 2+8+ed25519.PublicKeySize, len(raw))
	}
	var pk PublicKey
	copy(pk.KeyID[:], raw[2:10])
	pk.Key = ed25519.PublicKey(append([]byte(nil), raw[10:]...))
	return pk, nil
}

// EncodeSecretKey renders katyoti's (unencrypted) secret key file.
func EncodeSecretKey(sk SecretKey, comment string) string {
	if comment == "" {
		comment = "katyoti secret key — UNENCRYPTED, keep private"
	}
	blob := make([]byte, 0, 2+8+ed25519.PrivateKeySize)
	blob = append(blob, algLegacy...)
	blob = append(blob, sk.KeyID[:]...)
	blob = append(blob, sk.Key...)
	return "untrusted comment: " + comment + "\n" + base64.StdEncoding.EncodeToString(blob) + "\n"
}

// ParseSecretKey parses katyoti's secret key file.
func ParseSecretKey(data []byte) (SecretKey, error) {
	lines := payloadLines(data)
	if len(lines) == 0 {
		return SecretKey{}, fmt.Errorf("secret key: no key line")
	}
	raw, err := base64.StdEncoding.DecodeString(lines[0])
	if err != nil {
		return SecretKey{}, fmt.Errorf("secret key: base64: %w", err)
	}
	if len(raw) != 2+8+ed25519.PrivateKeySize {
		return SecretKey{}, fmt.Errorf("secret key: want %d bytes, got %d", 2+8+ed25519.PrivateKeySize, len(raw))
	}
	var sk SecretKey
	copy(sk.KeyID[:], raw[2:10])
	sk.Key = ed25519.PrivateKey(append([]byte(nil), raw[10:]...))
	return sk, nil
}

// Sign produces a minisign-compatible detached signature (`.minisig` content)
// over message, in legacy mode. trustedComment defaults if empty.
func Sign(message []byte, sk SecretKey, trustedComment string) string {
	if trustedComment == "" {
		trustedComment = "katyoti signed catalog"
	}
	sig := make([]byte, 0, 2+8+ed25519.SignatureSize)
	sig = append(sig, algLegacy...)
	sig = append(sig, sk.KeyID[:]...)
	sig = append(sig, ed25519.Sign(sk.Key, message)...)

	// Global signature binds the trusted comment: sign (ed25519_sig || comment).
	globalMsg := append(append([]byte(nil), sig[10:]...), []byte(trustedComment)...)
	globalSig := ed25519.Sign(sk.Key, globalMsg)

	var b strings.Builder
	b.WriteString("untrusted comment: signature from katyoti\n")
	b.WriteString(base64.StdEncoding.EncodeToString(sig) + "\n")
	b.WriteString("trusted comment: " + trustedComment + "\n")
	b.WriteString(base64.StdEncoding.EncodeToString(globalSig) + "\n")
	return b.String()
}

// Verify checks a minisign signature (`.minisig` bytes) over message against
// pk. Accepts legacy ("Ed", raw message) and prehashed ("ED",
// blake2b-512(message)) modes, so a signature produced by the `minisign` CLI
// either way verifies. Returns a non-nil error on ANY failure (the caller
// treats that as a hard install-refuse for pinned marketplaces).
func Verify(message, minisig []byte, pk PublicKey) error {
	lines := payloadLines(minisig)
	if len(lines) == 0 {
		return fmt.Errorf("signature: no signature line")
	}
	raw, err := base64.StdEncoding.DecodeString(lines[0])
	if err != nil {
		return fmt.Errorf("signature: base64: %w", err)
	}
	if len(raw) != 2+8+ed25519.SignatureSize {
		return fmt.Errorf("signature: want %d bytes, got %d", 2+8+ed25519.SignatureSize, len(raw))
	}
	alg := string(raw[0:2])
	var keyID [8]byte
	copy(keyID[:], raw[2:10])
	if keyID != pk.KeyID {
		return fmt.Errorf("signature: key id mismatch — signed by an untrusted key")
	}
	sig := raw[10:]

	var signed []byte
	switch alg {
	case algLegacy:
		signed = message
	case algPrehashed:
		h := blake2b.Sum512(message)
		signed = h[:]
	default:
		return fmt.Errorf("signature: unknown algorithm %q", alg)
	}
	if !ed25519.Verify(pk.Key, signed, sig) {
		return fmt.Errorf("signature: verification failed")
	}
	return nil
}

// payloadLines returns the non-empty, non-comment lines of a minisign-family
// file (key or signature), in order.
func payloadLines(data []byte) []string {
	var out []string
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" ||
			strings.HasPrefix(line, "untrusted comment:") ||
			strings.HasPrefix(line, "trusted comment:") {
			continue
		}
		out = append(out, line)
	}
	return out
}
