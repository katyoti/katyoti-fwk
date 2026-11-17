// SPDX-License-Identifier: Apache-2.0

package busauth

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nats-io/nkeys"
)

// A key once minted is kept (programs connected across a hub restart keep
// theirs) and the registry knows it; minted for another tenant, it is
// replaced and the old one no longer known.
func TestEnsure_KeepsAKeyAndReplacesOneOfAnotherTenant(t *testing.T) {
	dir := t.TempDir()
	a, err := Ensure(dir, KindSidecar, "ktw-x", "local")
	if err != nil {
		t.Fatal(err)
	}
	b, err := Ensure(dir, KindSidecar, "ktw-x", "local")
	if err != nil {
		t.Fatal(err)
	}
	if a.PublicKey() != b.PublicKey() {
		t.Fatal("a key minted again on a second Ensure")
	}
	reg := OpenRegistry(KeysDir(dir))
	if c, ok := reg.Lookup(a.PublicKey()); !ok || c != (Caller{KindSidecar, "ktw-x", "local"}) {
		t.Fatalf("registry lookup = %v %v", c, ok)
	}
	c, err := Ensure(dir, KindSidecar, "ktw-x", "acme")
	if err != nil {
		t.Fatal(err)
	}
	if c.PublicKey() == a.PublicKey() {
		t.Fatal("a key of another tenant kept")
	}
	if _, ok := reg.Lookup(a.PublicKey()); ok {
		t.Fatal("the replaced key is still known")
	}
	loaded, err := LoadCredential(CredentialPath(KeysDir(dir), KindSidecar, "ktw-x"))
	if err != nil || loaded == nil || loaded.PublicKey() != c.PublicKey() {
		t.Fatalf("LoadCredential = %v, %v", loaded, err)
	}
}

// A message verifies once; changed, stale or from an unknown key it does not.
func TestVerify(t *testing.T) {
	dir := t.TempDir()
	cli, err := Ensure(dir, KindCLI, KindCLI, "local")
	if err != nil {
		t.Fatal(err)
	}
	reg := cli.Registry()
	now := time.Now()
	h, err := cli.Sign("a.b", map[string][]string{"x": {"1"}, HeaderCause: {`{"kind":"person","id":"p"}`}}, []byte("body"), now)
	if err != nil {
		t.Fatal(err)
	}
	if c, err := Verify(reg, "a.b", h, []byte("body"), now); err != nil || c.Kind != KindCLI {
		t.Fatalf("Verify = %v, %v", c, err)
	}
	if _, err := VerifyCause(reg, h, now); err != nil {
		t.Fatalf("the CLI's cause attestation: %v", err)
	}
	if _, err := Verify(reg, "a.b", h, []byte("other"), now); !errors.Is(err, ErrBadSignature) {
		t.Errorf("a changed payload: %v", err)
	}
	if _, err := Verify(reg, "a.b", h, []byte("body"), now.Add(MessageWindow+time.Minute)); !errors.Is(err, ErrStale) {
		t.Errorf("a stale message: %v", err)
	}
	if _, err := Verify(OpenRegistry(t.TempDir()), "a.b", h, []byte("body"), now); !errors.Is(err, ErrUnknownKey) {
		t.Errorf("an unknown key: %v", err)
	}
	if _, err := Verify(reg, "a.b", map[string][]string{"x": {"1"}}, nil, now); !errors.Is(err, ErrUnsigned) {
		t.Errorf("an unsigned message: %v", err)
	}
	// A sidecar's key never attests a person's cause.
	side, err := Ensure(dir, KindSidecar, "ktw-s", "local")
	if err != nil {
		t.Fatal(err)
	}
	h2, err := side.Sign("a.b", map[string][]string{HeaderCause: {`{"kind":"person","id":"p"}`}}, nil, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyCause(reg, h2, now); err == nil {
		t.Error("a sidecar's call carried a person's cause attested")
	}
}

// fakeClock drives a registry's time in a test.
type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time       { return c.t }
func (c *fakeClock) pass(d time.Duration) { c.t = c.t.Add(d) }
func useClock(r *Registry) *fakeClock {
	c := &fakeClock{t: time.Unix(1_000_000, 0)}
	r.now = c.now
	return c
}
func writeKeys(t *testing.T, path string, keys map[string]Caller) {
	t.Helper()
	raw, _ := json.Marshal(keys)
	if err := writeAtomic(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

// A key another process minted just after this one read the registry is
// found on its next call past the floor (: a sidecar's key
// minted within the second after a reload was refused as « not one the hub
// minted »); the file is read again only when it changed.
func TestRegistry_FindsAKeyMintedJustAfterARead(t *testing.T) {
	dir := t.TempDir()
	if _, err := Ensure(dir, KindCLI, KindCLI, "local"); err != nil {
		t.Fatal(err)
	}
	keysDir := KeysDir(dir)
	reg := OpenRegistry(keysDir)
	clock := useClock(reg)
	if _, ok := reg.Lookup("UNKNOWN"); ok {
		t.Fatal("an unknown key found")
	}
	// Another process (the hub) mints a sidecar's key: the registry file
	// changes, this process's registry is not told.
	pair, err := nkeys.CreateUser()
	if err != nil {
		t.Fatal(err)
	}
	pub, _ := pair.PublicKey()
	regPath := filepath.Join(keysDir, registryFile)
	keys, err := readRegistry(regPath)
	if err != nil {
		t.Fatal(err)
	}
	want := Caller{KindSidecar, "ktw-acme", "local"}
	keys[pub] = want
	writeKeys(t, regPath, keys)
	clock.pass(statFloor)
	if c, ok := reg.Lookup(pub); !ok || c != want {
		t.Fatalf("a key minted just after a read: Lookup = %v %v", c, ok)
	}
	// An unknown key past the floor, the file unchanged: no read.
	reads := reg.reads
	clock.pass(statFloor)
	if _, ok := reg.Lookup("OTHER"); ok {
		t.Fatal("an unknown key found")
	}
	if reg.reads != reads {
		t.Errorf("an unchanged file was read again (%d reads, want %d)", reg.reads, reads)
	}
}

// A flood of distinct unknown keys inside the floor neither reads nor looks
// at the file: a key minted meanwhile is seen only past the floor.
func TestRegistry_AFloodOfUnknownKeysDoesNotReadTheFile(t *testing.T) {
	dir := t.TempDir()
	if _, err := Ensure(dir, KindCLI, KindCLI, "local"); err != nil {
		t.Fatal(err)
	}
	keysDir := KeysDir(dir)
	reg := OpenRegistry(keysDir)
	clock := useClock(reg)
	reg.Lookup("FIRST")
	reads, looks := reg.reads, reg.looks
	regPath := filepath.Join(keysDir, registryFile)
	keys, err := readRegistry(regPath)
	if err != nil {
		t.Fatal(err)
	}
	want := Caller{KindSidecar, "ktw-acme", "local"}
	for i := 0; i < 2000; i++ {
		key := "UNKNOWN-" + string(rune('A'+i%26)) + time.Duration(i).String()
		if i == 1000 {
			keys["LATE"] = want
			writeKeys(t, regPath, keys) // the file changes inside the floor
		}
		if _, ok := reg.Lookup(key); ok {
			t.Fatal("an unknown key found")
		}
		clock.pass(statFloor / 4000)
	}
	if reg.reads != reads || reg.looks != looks {
		t.Fatalf("2000 unknown keys inside the floor looked at the file %d times, read it %d times", reg.looks-looks, reg.reads-reads)
	}
	if _, ok := reg.Lookup("LATE"); ok {
		t.Fatal("a key minted inside the floor was seen before the floor passed")
	}
	clock.pass(statFloor)
	if c, ok := reg.Lookup("LATE"); !ok || c != want {
		t.Fatalf("a key minted inside the floor, past the floor: Lookup = %v %v", c, ok)
	}
	if reg.reads != reads+1 {
		t.Errorf("past the floor the changed file was read %d times, want 1", reg.reads-reads)
	}
}

// A read of the registry that fails for a moment (a lock held by a scanner
// or a rename on Windows) is not recorded as read: past the floor the
// unchanged file is read again and its keys found (: a failed
// read recorded as read left a lazily spawned sidecar refusing every call).
func TestRegistry_ReadsAgainAfterAFailedRead(t *testing.T) {
	dir := t.TempDir()
	cli, err := Ensure(dir, KindCLI, KindCLI, "local")
	if err != nil {
		t.Fatal(err)
	}
	reg := OpenRegistry(KeysDir(dir))
	clock := useClock(reg)
	reg.forget()
	fail := true
	reg.readFile = func(path string) ([]byte, error) {
		if fail {
			return nil, errors.New("the file is locked")
		}
		return os.ReadFile(path)
	}
	if _, ok := reg.Lookup(cli.PublicKey()); ok {
		t.Fatal("a key found while the read fails")
	}
	fail = false
	clock.pass(statFloor)
	if c, ok := reg.Lookup(cli.PublicKey()); !ok || c != cli.Caller {
		t.Fatalf("after a failed read, a key in the unchanged file: Lookup = %v %v", c, ok)
	}
}

// A mint whose read of the registry fails for a moment reads it again and
// never writes back a registry without the other programs' keys.
func TestEnsure_AFailedRegistryReadDropsNoKey(t *testing.T) {
	dir := t.TempDir()
	cli, err := Ensure(dir, KindCLI, KindCLI, "local")
	if err != nil {
		t.Fatal(err)
	}
	failures := 2
	mintReadFile = func(path string) ([]byte, error) {
		if failures > 0 {
			failures--
			return nil, errors.New("the file is locked")
		}
		return os.ReadFile(path)
	}
	t.Cleanup(func() { mintReadFile = os.ReadFile })
	if _, err := Ensure(dir, KindSidecar, "ktw-acme", "local"); err != nil {
		t.Fatal(err)
	}
	keys, err := readRegistry(filepath.Join(KeysDir(dir), registryFile))
	if err != nil {
		t.Fatal(err)
	}
	if keys[cli.PublicKey()] != cli.Caller {
		t.Errorf("a mint after a failed read dropped the CLI's key: %v", keys)
	}
}
