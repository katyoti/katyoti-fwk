// SPDX-License-Identifier: Apache-2.0

package proof

import (
	"crypto/hmac"
	"crypto/rand"
	"encoding/base32"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// The store's files, in the global config folder beside Profile.yaml - never
// in it: Profile.yaml is served to clients.
const (
	stateFile   = "Proof.sealed"
	journalFile = "proof-journal.jsonl"
	lockFile    = "Proof.lock"
)

// Backoff: this many wrong codes within the window slow the next attempts
// down until the window passes - never a hard lock a session could cause.
const (
	backoffFailures = 5
	backoffWindow   = 10 * time.Minute
)

// ErrNotEnrolled is a proof asked with no confirmed enrolment: it fails closed.
var ErrNotEnrolled = errors.New("no authenticator code is enrolled: the person enrols one first (katyoti profile code enrol)")

// state is what the sealed file holds.
type state struct {
	Secret     []byte `json:"secret"`
	Account    string `json:"account"`
	Confirmed  bool   `json:"confirmed"`
	EnrolledAt string `json:"enrolled_at"`
	LastStep   int64  `json:"last_step"`
}

// attempt is one line of the journal.
type attempt struct {
	At     string `json:"at"`
	Person string `json:"person,omitempty"`
	Act    string `json:"act,omitempty"`
	OK     bool   `json:"ok"`
	Reason string `json:"reason,omitempty"`
}

// Store is the person's enrolment in one folder.
type Store struct {
	Dir string
	Now func() time.Time
	// seal and unseal default to the platform's; tests replace them.
	seal, unseal func([]byte) ([]byte, error)
}

// New is the store in dir, sealed by the platform.
func New(dir string) *Store { return &Store{Dir: dir, Now: time.Now, seal: seal, unseal: unseal} }

// Status reports whether a code is enrolled and confirmed, and how it is sealed.
type Status struct {
	Enrolled  bool   `json:"enrolled"`
	Confirmed bool   `json:"confirmed"`
	Sealed    string `json:"sealed,omitempty"`
}

// Status reads the enrolment.
func (s *Store) Status() Status {
	st, err := s.read()
	if err != nil || st == nil {
		return Status{Sealed: sealedHere}
	}
	return Status{Enrolled: true, Confirmed: st.Confirmed, Sealed: sealedHere}
}

// Enrol draws a new secret for account, replacing any enrolment, unconfirmed
// until a code from the app confirms it. It answers the otpauth URI the app
// reads (as a QR code or typed) and the secret in base32 - the one time the
// secret leaves the store.
func (s *Store) Enrol(issuer, account string) (uri, secret string, err error) {
	raw := make([]byte, 20)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}
	err = s.locked(func() error {
		return s.write(&state{Secret: raw, Account: account, EnrolledAt: s.Now().UTC().Format(time.RFC3339)})
	})
	if err != nil {
		return "", "", err
	}
	b32 := strings.TrimRight(base32.StdEncoding.EncodeToString(raw), "=")
	label := url.PathEscape(issuer + ":" + account)
	q := url.Values{"secret": {b32}, "issuer": {issuer}, "algorithm": {"SHA1"}, "digits": {fmt.Sprint(digits)}, "period": {fmt.Sprint(period)}}
	return "otpauth://totp/" + label + "?" + q.Encode(), b32, nil
}

// Confirm checks a first code from the app and makes the enrolment usable.
func (s *Store) Confirm(c string) error {
	return s.locked(func() error {
		st, err := s.read()
		if err != nil {
			return err
		}
		if st == nil {
			return ErrNotEnrolled
		}
		step, ok := match(st.Secret, strings.TrimSpace(c), s.Now())
		if !ok {
			return errors.New("that code does not match: check the app's clock and type the current code")
		}
		st.Confirmed, st.LastStep = true, step
		return s.write(st)
	})
}

// Revoke removes the enrolment: every proof fails closed until a new one.
func (s *Store) Revoke() error {
	return s.locked(func() error {
		err := os.Remove(filepath.Join(s.Dir, stateFile))
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	})
}

// Prove checks code as the person's proof of act: once, within ±1 step, after
// the last step used, while not slowed down by wrong codes. Every attempt is
// journaled.
func (s *Store) Prove(person, act, c string) error {
	return s.locked(func() error {
		err := s.prove(person, act, strings.TrimSpace(c))
		a := attempt{At: s.Now().UTC().Format(time.RFC3339), Person: person, Act: act, OK: err == nil}
		if err != nil {
			a.Reason = err.Error()
		}
		s.journal(a)
		return err
	})
}

func (s *Store) prove(person, act, c string) error {
	if act == "" {
		return errors.New("a code proves one act: no act named")
	}
	if n := s.recentFailures(); n >= backoffFailures {
		return fmt.Errorf("%d wrong codes in the last %s: wait before trying again", n, backoffWindow)
	}
	st, err := s.read()
	if err != nil {
		return err
	}
	if st == nil || !st.Confirmed {
		return ErrNotEnrolled
	}
	if st.LastStep > 0 && hmac.Equal([]byte(code(st.Secret, st.LastStep, digits)), []byte(c)) {
		return errors.New("this code was already used: wait for the next code")
	}
	step, ok := match(st.Secret, c, s.Now())
	if !ok {
		return errors.New("wrong or expired code")
	}
	if step <= st.LastStep {
		return errors.New("this code was already used: wait for the next code")
	}
	st.LastStep = step
	return s.write(st)
}

// recentFailures counts the wrong codes of the backoff window.
func (s *Store) recentFailures() int {
	data, err := os.ReadFile(filepath.Join(s.Dir, journalFile))
	if err != nil {
		return 0
	}
	since := s.Now().Add(-backoffWindow)
	n := 0
	for _, line := range strings.Split(string(data), "\n") {
		var a attempt
		if json.Unmarshal([]byte(line), &a) != nil || a.OK {
			continue
		}
		if t, err := time.Parse(time.RFC3339, a.At); err == nil && t.After(since) {
			n++
		}
	}
	return n
}

func (s *Store) journal(a attempt) {
	line, _ := json.Marshal(a)
	f, err := os.OpenFile(filepath.Join(s.Dir, journalFile), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(line, '\n'))
}

// read is the unsealed state, nil when nothing is enrolled.
func (s *Store) read() (*state, error) {
	data, err := os.ReadFile(filepath.Join(s.Dir, stateFile))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	plain, err := s.unseal(data)
	if err != nil {
		return nil, fmt.Errorf("the enrolment cannot be unsealed (%v): enrol again", err)
	}
	var st state
	if err := json.Unmarshal(plain, &st); err != nil {
		return nil, fmt.Errorf("the enrolment is unreadable (%v): enrol again", err)
	}
	return &st, nil
}

func (s *Store) write(st *state) error {
	plain, err := json.Marshal(st)
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
	tmp := filepath.Join(s.Dir, stateFile+".tmp")
	if err := os.WriteFile(tmp, sealed, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(s.Dir, stateFile))
}

// locked runs fn holding the store's lock file: processes of this user prove
// one at a time, so a code is never spent twice.
func (s *Store) locked(fn func() error) error {
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return err
	}
	path := filepath.Join(s.Dir, lockFile)
	for i := 0; ; i++ {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			f.Close()
			break
		}
		if st, serr := os.Stat(path); serr == nil && time.Since(st.ModTime()) > 30*time.Second {
			_ = os.Remove(path) // left by a process that died
			continue
		}
		if i > 100 {
			return errors.New("the authenticator store is busy: try again")
		}
		time.Sleep(20 * time.Millisecond)
	}
	defer os.Remove(path)
	return fn()
}

// Seal and Unseal are the platform's sealing for this user (Windows DPAPI),
// for the person's other secrets kept beside the code's (their passkeys).
func Seal(b []byte) ([]byte, error)   { return seal(b) }
func Unseal(b []byte) ([]byte, error) { return unseal(b) }
