// SPDX-License-Identifier: Apache-2.0

// Package busauth is who may speak on a universe's bus and who a message is
// from: the hub mints a key per client kind (the GUI, a CLI, the
// hub itself) and per sidecar, the embedded server refuses a connection
// without one, every message a keyed connection sends carries its signature,
// and the serving side reads the caller off that signature - the stamp - never
// off a header the caller wrote.
//
// The keys live in the universe's data folder (<DataDir>/hub/keys/): a seed
// per program (its secret, readable by the OS user only) and the registry, the
// public key of each with whose it is (kind, name, tenant). The registry is
// what a receiver checks a signature against; the hub is its one writer.
//
// A person's cause is attested apart: a person's client (the GUI, a CLI) signs
// the cause header it sends, and a program relaying the call forwards that
// attestation unchanged. A call whose cause says « person » without an
// attestation of a person's client is refused - a sidecar or an agent's
// process cannot make up the person's act; it can only carry one it was given.
//
// A neutral leaf: the nkeys library and the standard library only - no NATS
// client, so the transport, the KTSP messagebus and the hub client share it.
package busauth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/nats-io/nkeys"
)

// The client kinds a key is minted for.
const (
	// KindHub is the hub's own in-process connection.
	KindHub = "hub"
	// KindGUI is the desktop GUI's connection: a person's client.
	KindGUI = "gui"
	// KindCLI is a command line's connection (katyoti, ktw-agents, a
	// sidecar's own CLI): a person's client - « person, unproven » until
	// iam tells an agent's command from the person's.
	KindCLI = "cli"
	// KindSidecar is a member the hub spawned: it serves slots and relays
	// the calls it serves, never originates a person's act.
	KindSidecar = "sidecar"
)

// PersonClient reports whether a key of this kind may attest a person's
// cause: the GUI and a CLI, the programs a person types or clicks in.
func PersonClient(kind string) bool { return kind == KindGUI || kind == KindCLI }

// Wire headers the transport writes and reads. HeaderCause is the KTSP cause
// header (the messagebus package names it from here, one spelling).
const (
	// HeaderKey is the sender's public key.
	HeaderKey = "ktw.meta.key"
	// HeaderSignedAt is when the message was signed (unix milliseconds).
	HeaderSignedAt = "ktw.meta.signedAt"
	// HeaderSig is the signature of the message: subject, every other
	// header, the payload's digest.
	HeaderSig = "ktw.meta.sig"
	// HeaderCause is the caller's cause (internal/core/cause, JSON).
	HeaderCause = "ktw.meta.cause"
	// HeaderCauseSig attests HeaderCause: `<public key>.<signedAt>.<sig>`,
	// signed by the person's client that set the cause.
	HeaderCauseSig = "ktw.meta.causeSig"
)

// MessageWindow is how far a message's signing time may be from the
// receiver's clock; CauseWindow how long a person's attested cause may be
// relayed after it was signed (a call a person started keeps its person
// through the work it sets off for this long).
const (
	MessageWindow = 10 * time.Minute
	CauseWindow   = 12 * time.Hour
)

// Caller is who sent a message, as the key that signed it says: its kind,
// its name (the sidecar's, or the kind's for a client) and its tenant - the
// tenant comes from the key, never from a header.
type Caller struct {
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	Tenant string `json:"tenant"`
}

// String is `<kind>:<name>`, the name the server reports the connection by.
func (c Caller) String() string { return c.Kind + ":" + c.Name }

// IsZero reports an unset caller.
func (c Caller) IsZero() bool { return c.Kind == "" && c.Name == "" }

// Errors a verification answers.
var (
	// ErrOpenBus: the connection the message came on checks no keys (a test
	// bus, or a hub of a version before keys) - there is no caller to read.
	ErrOpenBus = errors.New("busauth: the bus checks no keys")
	// ErrUnsigned: a keyed bus carried a message with no signature.
	ErrUnsigned = errors.New("busauth: the message carries no signature")
	// ErrUnknownKey: the signing key is not in the universe's registry.
	ErrUnknownKey = errors.New("busauth: the signing key is not one the hub minted")
	// ErrBadSignature: the signature does not match the message.
	ErrBadSignature = errors.New("busauth: the signature does not match the message")
	// ErrStale: the message was signed too far from now.
	ErrStale = errors.New("busauth: the message was signed too far from now")
)

// Lookup answers whose a public key is.
type Lookup interface {
	Lookup(publicKey string) (Caller, bool)
}

// Stamped is a received message the transport can tell the sender of. A
// message of a bus that checks no keys answers ErrOpenBus.
type Stamped interface {
	// Caller verifies the message's signature and answers whose key signed it.
	Caller() (Caller, error)
	// CauseAttester verifies the attestation of the message's cause header
	// and answers whose key signed it - a person's client's, or an error.
	CauseAttester() (Caller, error)
}

// CallerOf is the verified caller of msg; ErrOpenBus when msg comes from a bus
// that checks no keys (or is not a transport message at all).
func CallerOf(msg any) (Caller, error) {
	s, ok := msg.(Stamped)
	if !ok {
		return Caller{}, ErrOpenBus
	}
	return s.Caller()
}

// ---------------------------------------------------------------------------
// Signing and verifying a message
// ---------------------------------------------------------------------------

// canonical is what a message's signature covers: the subject, every header
// but the signature (sorted, each value in order), the payload's digest.
func canonical(subject string, header map[string][]string, payload []byte) []byte {
	keys := make([]string, 0, len(header))
	for k := range header {
		if k == HeaderSig {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString("ktw-msg\n")
	b.WriteString(subject)
	b.WriteByte('\n')
	for _, k := range keys {
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(strings.Join(header[k], "\x00"))
		b.WriteByte('\n')
	}
	sum := sha256.Sum256(payload)
	b.WriteString(hex.EncodeToString(sum[:]))
	return []byte(b.String())
}

// Sign adds the credential's signature to header (created when nil) for a
// message of subject and payload, and an attestation of its cause header when
// the credential is a person's client's and none is carried yet. It never
// keeps a signature header the caller wrote.
func (c *Credential) Sign(subject string, header map[string][]string, payload []byte, now time.Time) (map[string][]string, error) {
	if header == nil {
		header = map[string][]string{}
	}
	delete(header, HeaderSig)
	at := strconv.FormatInt(now.UnixMilli(), 10)
	if v := header[HeaderCause]; len(v) > 0 && v[0] != "" && len(header[HeaderCauseSig]) == 0 && PersonClient(c.Caller.Kind) {
		att, err := c.attest(v[0], at)
		if err != nil {
			return nil, err
		}
		header[HeaderCauseSig] = []string{att}
	}
	header[HeaderKey] = []string{c.public}
	header[HeaderSignedAt] = []string{at}
	sig, err := c.pair.Sign(canonical(subject, header, payload))
	if err != nil {
		return nil, fmt.Errorf("busauth: sign: %w", err)
	}
	header[HeaderSig] = []string{base64.RawURLEncoding.EncodeToString(sig)}
	return header, nil
}

// Verify checks a received message's signature against the registry and
// answers whose key made it.
func Verify(reg Lookup, subject string, header map[string][]string, payload []byte, now time.Time) (Caller, error) {
	key, at, sig := first(header, HeaderKey), first(header, HeaderSignedAt), first(header, HeaderSig)
	if key == "" || at == "" || sig == "" {
		return Caller{}, ErrUnsigned
	}
	caller, ok := reg.Lookup(key)
	if !ok {
		return Caller{}, fmt.Errorf("%w: %s", ErrUnknownKey, key)
	}
	if err := fresh(at, now, MessageWindow); err != nil {
		return Caller{}, err
	}
	if err := verifySig(key, sig, canonical(subject, header, payload)); err != nil {
		return Caller{}, err
	}
	return caller, nil
}

// VerifyCause checks the attestation of a message's cause header: signed by a
// key of the registry, a person's client's, over this very cause, within
// CauseWindow. It answers whose key attested.
func VerifyCause(reg Lookup, header map[string][]string, now time.Time) (Caller, error) {
	raw := first(header, HeaderCause)
	att := first(header, HeaderCauseSig)
	if att == "" {
		return Caller{}, errors.New("busauth: a person's cause carries no attestation of a person's client")
	}
	parts := strings.SplitN(att, ".", 3)
	if len(parts) != 3 {
		return Caller{}, errors.New("busauth: the cause attestation is malformed")
	}
	key, at, sig := parts[0], parts[1], parts[2]
	caller, ok := reg.Lookup(key)
	if !ok {
		return Caller{}, fmt.Errorf("%w: %s (the cause's attester)", ErrUnknownKey, key)
	}
	if !PersonClient(caller.Kind) {
		return Caller{}, fmt.Errorf("busauth: the cause was attested by %s, not a person's client", caller)
	}
	if err := fresh(at, now, CauseWindow); err != nil {
		return Caller{}, err
	}
	if err := verifySig(key, sig, causePayload(raw, at)); err != nil {
		return Caller{}, err
	}
	return caller, nil
}

func causePayload(raw, at string) []byte { return []byte("ktw-cause\n" + at + "\n" + raw) }

func (c *Credential) attest(raw, at string) (string, error) {
	sig, err := c.pair.Sign(causePayload(raw, at))
	if err != nil {
		return "", fmt.Errorf("busauth: attest the cause: %w", err)
	}
	return c.public + "." + at + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

func verifySig(key, sig string, payload []byte) error {
	raw, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil {
		return ErrBadSignature
	}
	pub, err := nkeys.FromPublicKey(key)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnknownKey, err)
	}
	if err := pub.Verify(payload, raw); err != nil {
		return ErrBadSignature
	}
	return nil
}

func fresh(at string, now time.Time, window time.Duration) error {
	ms, err := strconv.ParseInt(at, 10, 64)
	if err != nil {
		return ErrStale
	}
	d := now.Sub(time.UnixMilli(ms))
	if d < 0 {
		d = -d
	}
	if d > window {
		return fmt.Errorf("%w (%s off, at most %s)", ErrStale, d.Round(time.Second), window)
	}
	return nil
}

func first(h map[string][]string, k string) string {
	if v := h[k]; len(v) > 0 {
		return v[0]
	}
	return ""
}

// VerifyNonce checks a connection's signature of the server's nonce with the
// public key it presents - the embedded server's authentication of a client.
func VerifyNonce(publicKey string, nonce []byte, sig string) bool {
	raw, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil {
		if raw, err = base64.StdEncoding.DecodeString(sig); err != nil {
			return false
		}
	}
	pub, err := nkeys.FromPublicKey(publicKey)
	if err != nil {
		return false
	}
	return pub.Verify(nonce, raw) == nil
}

// ---------------------------------------------------------------------------
// The caller on a context
// ---------------------------------------------------------------------------

type callerKey struct{}

// WithCaller is ctx carrying the verified caller of the call it serves.
func WithCaller(ctx context.Context, c Caller) context.Context {
	return context.WithValue(ctx, callerKey{}, c)
}

// CallerFrom is the verified caller ctx serves, false when none (an in-process
// call, or a bus that checks no keys).
func CallerFrom(ctx context.Context) (Caller, bool) {
	c, ok := ctx.Value(callerKey{}).(Caller)
	return c, ok && !c.IsZero()
}

type attestationKey struct{}

type attestation struct{ cause, sig string }

// WithCauseAttestation is ctx carrying the cause header a served call came
// with and its attestation, so a call made while serving it forwards the
// person's attestation unchanged.
func WithCauseAttestation(ctx context.Context, rawCause, sig string) context.Context {
	if rawCause == "" || sig == "" {
		return ctx
	}
	return context.WithValue(ctx, attestationKey{}, attestation{rawCause, sig})
}

// CauseAttestationFrom is the cause header the served call came with and its
// attestation, false when ctx carries none.
func CauseAttestationFrom(ctx context.Context) (rawCause, sig string, ok bool) {
	a, ok := ctx.Value(attestationKey{}).(attestation)
	if !ok {
		return "", "", false
	}
	return a.cause, a.sig, true
}
