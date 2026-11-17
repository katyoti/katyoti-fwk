// SPDX-License-Identifier: Apache-2.0

// Package passkey is the person's passkey: registered
// once from their phone on the approval page, then signing, with Face ID or a
// fingerprint, a challenge bound to one act of the person - the one-tap phone
// approval. WebAuthn is verified by github.com/go-webauthn/webauthn; the
// credentials are sealed for this user (Windows DPAPI) beside the
// authenticator code's store, apart from Profile.yaml; challenges and verified
// approvals live in memory in the process serving the approval page - short,
// single use. The relying party is the page's own HTTPS origin (the PC's
// tailnet name, tailscale serve).
package passkey

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

// credsFile holds the people's credentials, sealed.
const credsFile = "Passkeys.sealed"

// pendingTTL is how long a challenge waits for the phone; verifiedTTL how
// long a verified approval can be checked.
const (
	pendingTTL  = 5 * time.Minute
	verifiedTTL = 5 * time.Minute
)

// person is a WebAuthn user: the person's handle and their credentials.
type person struct {
	ID    string                `json:"id"`
	Creds []webauthn.Credential `json:"credentials"`
}

func (p *person) WebAuthnID() []byte                         { return []byte(p.ID) }
func (p *person) WebAuthnName() string                       { return p.ID }
func (p *person) WebAuthnDisplayName() string                { return p.ID }
func (p *person) WebAuthnCredentials() []webauthn.Credential { return p.Creds }

// ceremony is a challenge waiting for the phone: a registration, or an
// approval of one act.
type ceremony struct {
	register bool
	person   string
	act      string
	origin   string
	session  webauthn.SessionData
	expires  time.Time
	verified bool
	spent    bool
}

// Store is the people's passkeys in one folder and the ceremonies of this
// process.
type Store struct {
	Dir          string
	Now          func() time.Time
	seal, unseal func([]byte) ([]byte, error)

	mu    sync.Mutex
	cer   map[string]*ceremony
	files sync.Mutex
}

// New is the store in dir, sealed with seal and unseal (the platform's).
func New(dir string, seal, unseal func([]byte) ([]byte, error)) *Store {
	return &Store{Dir: dir, Now: time.Now, seal: seal, unseal: unseal, cer: map[string]*ceremony{}}
}

// relyingParty is WebAuthn for the page at origin: its host is the RP ID.
func relyingParty(origin string) (*webauthn.WebAuthn, error) {
	u, err := url.Parse(origin)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" {
		return nil, fmt.Errorf("the approval page's origin must be https://<host>: got %q", origin)
	}
	return webauthn.New(&webauthn.Config{RPID: u.Hostname(), RPDisplayName: "KaTyoti", RPOrigins: []string{origin}})
}

func newHandle() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// BeginRegistration starts registering a passkey for the person at the page's
// origin: the handle and the options the phone's navigator.credentials.create
// takes.
func (s *Store) BeginRegistration(personID, origin string) (string, json.RawMessage, error) {
	rp, err := relyingParty(origin)
	if err != nil {
		return "", nil, err
	}
	p, err := s.person(personID)
	if err != nil {
		return "", nil, err
	}
	creation, session, err := rp.BeginRegistration(p,
		webauthn.WithAuthenticatorSelection(protocol.AuthenticatorSelection{
			ResidentKey: protocol.ResidentKeyRequirementPreferred, UserVerification: protocol.VerificationRequired}),
		webauthn.WithConveyancePreference(protocol.PreferNoAttestation),
		webauthn.WithExclusions(webauthn.Credentials(p.Creds).CredentialDescriptors()))
	if err != nil {
		return "", nil, err
	}
	options, err := json.Marshal(creation)
	if err != nil {
		return "", nil, err
	}
	h := newHandle()
	s.mu.Lock()
	s.cer[h] = &ceremony{register: true, person: personID, origin: origin, session: *session, expires: s.Now().Add(pendingTTL)}
	s.mu.Unlock()
	return h, options, nil
}

// FinishRegistration verifies the phone's answer and stores its credential.
func (s *Store) FinishRegistration(handle string, response []byte) error {
	c, err := s.take(handle, true)
	if err != nil {
		return err
	}
	rp, err := relyingParty(c.origin)
	if err != nil {
		return err
	}
	parsed, err := protocol.ParseCredentialCreationResponseBody(bytes.NewReader(response))
	if err != nil {
		return fmt.Errorf("the phone's answer is unreadable: %v", err)
	}
	p, err := s.person(c.person)
	if err != nil {
		return err
	}
	cred, err := rp.CreateCredential(p, c.session, parsed)
	if err != nil {
		return fmt.Errorf("the passkey is refused: %v", err)
	}
	return s.edit(func(all map[string]*person) {
		q := all[c.person]
		if q == nil {
			q = &person{ID: c.person}
			all[c.person] = q
		}
		q.Creds = append(q.Creds, *cred)
	})
}

// BeginApproval starts the approval of one act of the person: the handle and
// the options the phone's navigator.credentials.get takes. Refused when the
// person has no passkey.
func (s *Store) BeginApproval(personID, act, origin string) (string, json.RawMessage, error) {
	if act == "" {
		return "", nil, errors.New("an approval is of one act: no act named")
	}
	rp, err := relyingParty(origin)
	if err != nil {
		return "", nil, err
	}
	p, err := s.person(personID)
	if err != nil {
		return "", nil, err
	}
	if len(p.Creds) == 0 {
		return "", nil, fmt.Errorf("%s has no passkey: they register one first on the approval page", personID)
	}
	assertion, session, err := rp.BeginLogin(p, webauthn.WithUserVerification(protocol.VerificationRequired))
	if err != nil {
		return "", nil, err
	}
	options, err := json.Marshal(assertion)
	if err != nil {
		return "", nil, err
	}
	h := newHandle()
	s.mu.Lock()
	s.cer[h] = &ceremony{person: personID, act: act, origin: origin, session: *session, expires: s.Now().Add(pendingTTL)}
	s.mu.Unlock()
	return h, options, nil
}

// FinishApproval verifies the phone's signature over the act's challenge -
// user verification required, the sign counter rising - and marks the
// approval verified, to be checked once.
func (s *Store) FinishApproval(handle string, response []byte) error {
	s.mu.Lock()
	c := s.cer[handle]
	s.mu.Unlock()
	switch {
	case c == nil || c.register:
		return errors.New("no approval waits under this handle")
	case c.verified || c.spent:
		return errors.New("this approval was already given")
	case s.Now().After(c.expires):
		return errors.New("the approval expired: ask again")
	}
	rp, err := relyingParty(c.origin)
	if err != nil {
		return err
	}
	parsed, err := protocol.ParseCredentialRequestResponseBody(bytes.NewReader(response))
	if err != nil {
		return fmt.Errorf("the phone's answer is unreadable: %v", err)
	}
	p, err := s.person(c.person)
	if err != nil {
		return err
	}
	cred, err := rp.ValidateLogin(p, c.session, parsed)
	if err != nil {
		return fmt.Errorf("the approval is refused: %v", err)
	}
	if cred.Authenticator.CloneWarning {
		return errors.New("the approval is refused: the passkey's counter went back (a cloned key?)")
	}
	if err := s.edit(func(all map[string]*person) {
		if q := all[c.person]; q != nil {
			for i := range q.Creds {
				if bytes.Equal(q.Creds[i].ID, cred.ID) {
					q.Creds[i].Authenticator.SignCount = cred.Authenticator.SignCount
				}
			}
		}
	}); err != nil {
		return err
	}
	s.mu.Lock()
	c.verified, c.expires = true, s.Now().Add(verifiedTTL)
	s.mu.Unlock()
	return nil
}

// Check answers once whether handle is an approval verified for act and
// person, then spends it.
func (s *Store) Check(handle, act, personID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.cer[handle]
	switch {
	case c == nil || c.register:
		return errors.New("no such approval")
	case c.spent:
		return errors.New("this approval was already used")
	case !c.verified:
		return errors.New("not approved on the phone yet")
	case c.act != act || c.person != personID:
		return errors.New("approved for another act or person")
	case s.Now().After(c.expires):
		delete(s.cer, handle)
		return errors.New("the approval expired: ask again")
	}
	c.spent = true
	delete(s.cer, handle)
	return nil
}

// Count is how many passkeys the person registered.
func (s *Store) Count(personID string) int {
	p, err := s.person(personID)
	if err != nil {
		return 0
	}
	return len(p.Creds)
}

// Revoke removes the person's passkeys: no phone approval passes until one is
// registered again.
func (s *Store) Revoke(personID string) error {
	return s.edit(func(all map[string]*person) { delete(all, personID) })
}

// take removes and answers a waiting registration.
func (s *Store) take(handle string, register bool) (*ceremony, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.cer[handle]
	if c == nil || c.register != register {
		return nil, errors.New("nothing waits under this handle")
	}
	delete(s.cer, handle)
	if s.Now().After(c.expires) {
		return nil, errors.New("it expired: start again")
	}
	return c, nil
}

func (s *Store) person(id string) (*person, error) {
	all, err := s.read()
	if err != nil {
		return nil, err
	}
	if p := all[id]; p != nil {
		return p, nil
	}
	return &person{ID: id}, nil
}

func (s *Store) read() (map[string]*person, error) {
	all := map[string]*person{}
	data, err := os.ReadFile(filepath.Join(s.Dir, credsFile))
	if errors.Is(err, fs.ErrNotExist) {
		return all, nil
	}
	if err != nil {
		return nil, err
	}
	plain, err := s.unseal(data)
	if err != nil {
		return nil, fmt.Errorf("the passkeys cannot be unsealed (%v): register again", err)
	}
	if err := json.Unmarshal(plain, &all); err != nil {
		return nil, fmt.Errorf("the passkeys are unreadable (%v): register again", err)
	}
	return all, nil
}

func (s *Store) edit(fn func(map[string]*person)) error {
	s.files.Lock()
	defer s.files.Unlock()
	all, err := s.read()
	if err != nil {
		return err
	}
	fn(all)
	plain, err := json.Marshal(all)
	if err != nil {
		return err
	}
	sealed, err := s.seal(plain)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return err
	}
	tmp := filepath.Join(s.Dir, credsFile+".tmp")
	if err := os.WriteFile(tmp, sealed, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(s.Dir, credsFile))
}
