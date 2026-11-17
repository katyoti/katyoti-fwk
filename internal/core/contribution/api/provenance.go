// SPDX-License-Identifier: Apache-2.0

package contribapi

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"
)

// provenance.go — asserting where a contribution came from.
//
// # Signing identity, not bytes
//
// Every layer is content-addressed, so a layer's hash IS its identity and a
// contribution's identity is its URI plus the set of its layer hashes. A
// signature therefore covers a few hundred bytes of identity rather than the
// content, which may be arbitrarily large — the same assertion over a 4 GB
// dataset costs exactly what it costs over a 2 KB skill.
//
// Verifying is one signature check plus the hash comparison a consumer already
// performs when it reads content. Tampering with any layer changes that layer's
// hash, which changes the identity, which invalidates the signature.
//
// # An assertion, not a requirement
//
// An unsigned contribution is valid and usable. Provenance is something a
// publisher MAY attach; making it mandatory would be an enforcement policy, and
// enforcement — which signatures are required, who issues keys, what a
// component refuses to load — belongs to the security model, not here. This
// package's whole obligation is that origin can be EXPRESSED and CHECKED.

// Provenance is an optional assertion of a contribution's origin.
type Provenance struct {
	// Signature is over IdentityDigest, in whatever encoding the signing
	// facility produces. Opaque here: this package states what is signed,
	// not how, so the algorithm can change without the envelope changing.
	Signature string `json:"signature,omitempty"`

	// KeyID names the public key that verifies Signature. A consumer needs
	// to know WHICH key to check against; which keys it trusts is policy.
	KeyID string `json:"keyId,omitempty"`

	// Publisher is a human-readable claim about who signed. Informational
	// only — it is inside the signed payload's comment, not a substitute
	// for verifying the signature.
	Publisher string `json:"publisher,omitempty"`
}

// IdentityDigest returns the canonical content address of this contribution's
// IDENTITY: its URI, its kind, and every hash it carries.
//
// Canonical means two producers of the same contribution compute the same
// digest — layers are sorted by name, and every field is length-prefixed so no
// combination of values can be re-read as a different combination. Without
// that, a signature would verify against content it did not cover.
//
// Every layer is covered, including `data` — a contribution's structured
// content is a layer like any other, so signing the layer set signs the
// content. Before that was true this function had a separate body clause, and
// the reason it could be removed is the reason c22 existed: there was one kind
// of content held in two places.
func (c Contribution) IdentityDigest() string {
	h := sha256.New()
	write := func(parts ...string) {
		for _, p := range parts {
			// Length-prefixed: "ab"+"c" and "a"+"bc" must not hash alike.
			h.Write([]byte(strconv.Itoa(len(p))))
			h.Write([]byte{':'})
			h.Write([]byte(p))
			h.Write([]byte{'\n'})
		}
	}

	write("uri", c.ResolvedURI())
	write("kind", c.Type)
	names := make([]string, 0, len(c.Layers))
	for name := range c.Layers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		l := c.Layers[name]
		write("layer", name, l.Hash, strconv.FormatInt(l.Size, 10))
	}

	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

// Signed reports whether this contribution carries a provenance assertion.
// Says nothing about whether it VERIFIES — that needs a key, and needing a key
// is what keeps trust decisions out of the envelope.
func (c Contribution) Signed() bool {
	return c.Provenance != nil && strings.TrimSpace(c.Provenance.Signature) != ""
}

// === Execution class ===
//
// A kind whose content is EXECUTED carries a different exposure from one whose
// content is READ. A reference reaches an agent's context and can influence
// model behaviour; a script reaches the machine and can run code. The model
// refuses to conflate them, so a consumer can apply a different posture to each — and
// so a component that serves content it did not author is able to decline to
// execute it.

// Execution is how a kind's content is consumed.
type Execution string

const (
	// ExecutionDeclarative is content that is READ — references, personas,
	// schemas, configuration. The default: a kind that does not
	// say is treated as declarative, because the safe assumption about an
	// unknown kind is that nothing will run it.
	ExecutionDeclarative Execution = "declarative"

	// ExecutionExecutable is content that RUNS, names something to run, or
	// widens rights — tasks, connectors, hooks, workflows, policies, gate
	// requirements, capabilities, permission profiles, skills, object types,
	// display engines, config programs. An untrusted
	// origin may not override an entry of such a kind. Declared explicitly: a
	// kind cannot become executable by accident.
	ExecutionExecutable Execution = "executable"
)

// ExecutionOf normalises a kind declaration's `execution:` field.
// Anything unrecognised — including empty — resolves to declarative.
func ExecutionOf(s string) Execution {
	if strings.TrimSpace(strings.ToLower(s)) == string(ExecutionExecutable) {
		return ExecutionExecutable
	}
	return ExecutionDeclarative
}
