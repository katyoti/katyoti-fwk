// SPDX-License-Identifier: Apache-2.0

package busauth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/nats-io/nkeys"
)

// KeysDir is where a universe's bus keys live: <DataDir>/hub/keys.
func KeysDir(dataDir string) string { return filepath.Join(dataDir, "hub", "keys") }

// KeyEnv names the variable a member the hub spawned reads the path of its
// bus key from (the path, never the key).
const KeyEnv = "KTW_BUS_KEY"

// FromEnv is the key KeyEnv names; nil, nil when it names none.
func FromEnv() (*Credential, error) {
	path := os.Getenv(KeyEnv)
	if path == "" {
		return nil, nil
	}
	cred, err := LoadCredential(path)
	if err == nil && cred == nil {
		err = fmt.Errorf("busauth: %s names %s, which holds no key", KeyEnv, path)
	}
	return cred, err
}

// registryFile names the registry in a keys folder.
const registryFile = "registry.json"

// CredentialPath is the seed file of a program's key in a keys folder:
// `<kind>.key` for a client kind, `sidecar-<name>.key` for a sidecar.
func CredentialPath(dir, kind, name string) string {
	if kind == KindSidecar {
		return filepath.Join(dir, "sidecar-"+name+".key")
	}
	return filepath.Join(dir, kind+".key")
}

// Credential is a program's key: whose it is and its secret. It signs the
// connection's nonce and every message the connection sends.
type Credential struct {
	Caller Caller
	dir    string
	public string
	pair   nkeys.KeyPair
}

// PublicKey is the key's public half, the one the registry names.
func (c *Credential) PublicKey() string { return c.public }

// SignNonce signs the server's nonce (the connection's authentication).
func (c *Credential) SignNonce(nonce []byte) ([]byte, error) { return c.pair.Sign(nonce) }

// Registry is the registry of the keys folder the credential came from: what
// the connection checks the messages it receives against.
func (c *Credential) Registry() *Registry { return OpenRegistry(c.dir) }

// credentialFile is a seed file: whose key it is and the seed.
type credentialFile struct {
	Caller
	Seed string `json:"seed"`
}

// LoadCredential reads the key at path; nil, nil when there is none.
func LoadCredential(path string) (*Credential, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("busauth: read the key %s: %w", path, err)
	}
	var f credentialFile
	if err := json.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("busauth: the key %s does not decode: %w", path, err)
	}
	pair, err := nkeys.FromSeed([]byte(f.Seed))
	if err != nil {
		return nil, fmt.Errorf("busauth: the key %s holds no valid seed: %w", path, err)
	}
	pub, err := pair.PublicKey()
	if err != nil {
		return nil, err
	}
	return &Credential{Caller: f.Caller, dir: filepath.Dir(path), public: pub, pair: pair}, nil
}

// LoadClient reads the key of a client kind in a universe's data folder; nil,
// nil when the hub minted none (a hub of a version before keys).
func LoadClient(dataDir, kind string) (*Credential, error) {
	return LoadCredential(CredentialPath(KeysDir(dataDir), kind, kind))
}

// ---------------------------------------------------------------------------
// The registry
// ---------------------------------------------------------------------------

// Registry is a keys folder's registry: the public key of every key the hub
// minted, with whose it is. Read from its file, and read again only when a key
// it does not know is asked and the file changed since the last read (another
// identity, size or modification time: a mint renames a new file in place), so
// a key minted since the last read (by the hub, another process) is found on
// its next call. The file is looked at no more than once per statFloor: a
// flood of unknown keys costs neither a read nor a stat per attempt. A read
// that fails (a lock held by a scanner or a rename on Windows, a torn file)
// is not recorded as read: the next look past the floor reads again, since
// nothing retries a refused call.
type Registry struct {
	path     string
	now      func() time.Time
	readFile func(string) ([]byte, error)

	mu      sync.Mutex
	keys    map[string]Caller
	read    os.FileInfo // the file as it was when last read well; nil when absent or not read well
	checked time.Time   // when the file was last looked at
	reads   int         // how many times the file was read
	looks   int         // how many times the file was looked at
}

// statFloor is the least time between two looks at the registry file.
const statFloor = 50 * time.Millisecond

var (
	registriesMu sync.Mutex
	registries   = map[string]*Registry{}
)

// OpenRegistry is the registry of the keys folder dir, one per folder in a
// process.
func OpenRegistry(dir string) *Registry {
	path := filepath.Join(dir, registryFile)
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	registriesMu.Lock()
	defer registriesMu.Unlock()
	if r, ok := registries[path]; ok {
		return r
	}
	r := &Registry{path: path, now: time.Now, readFile: os.ReadFile}
	registries[path] = r
	return r
}

// Lookup answers whose publicKey is.
func (r *Registry) Lookup(publicKey string) (Caller, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if c, ok := r.keys[publicKey]; ok {
		return c, true
	}
	now := r.now()
	if r.keys != nil && now.Sub(r.checked) < statFloor {
		return Caller{}, false
	}
	r.checked = now
	r.looks++
	fi, err := os.Stat(r.path)
	if err != nil {
		fi = nil
	}
	if r.keys != nil && sameFile(r.read, fi) {
		return Caller{}, false
	}
	r.reloadLocked(fi)
	c, ok := r.keys[publicKey]
	return c, ok
}

// sameFile reports a file unchanged between two looks (both absent counts).
func sameFile(a, b os.FileInfo) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return os.SameFile(a, b) && a.Size() == b.Size() && a.ModTime().Equal(b.ModTime())
}

// forget drops what the registry read, so the next Lookup reads its file.
func (r *Registry) forget() {
	r.mu.Lock()
	r.keys, r.read, r.checked = nil, nil, time.Time{}
	r.mu.Unlock()
}

// reloadLocked reads the file, fi being how it looked just before. A read
// that fails or does not decode keeps the keys read before and records no
// read, so the next look past the floor reads the file again even when it
// did not change.
func (r *Registry) reloadLocked(fi os.FileInfo) {
	r.reads++
	keys, err := readRegistryWith(r.readFile, r.path)
	if err != nil {
		r.read = nil
		if r.keys == nil {
			r.keys = map[string]Caller{}
		}
		return
	}
	r.read = fi
	r.keys = keys
}

// errUndecodable marks a registry file that was read but does not decode.
var errUndecodable = errors.New("busauth: the registry does not decode")

func readRegistry(path string) (map[string]Caller, error) {
	return readRegistryWith(os.ReadFile, path)
}

func readRegistryWith(readFile func(string) ([]byte, error), path string) (map[string]Caller, error) {
	raw, err := readFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]Caller{}, nil
	}
	if err != nil {
		return nil, err
	}
	keys := map[string]Caller{}
	if err := json.Unmarshal(raw, &keys); err != nil {
		return nil, fmt.Errorf("%w: %s: %v", errUndecodable, path, err)
	}
	return keys, nil
}

// ---------------------------------------------------------------------------
// Minting - the hub only
// ---------------------------------------------------------------------------

var mintMu sync.Mutex

// Ensure is the key of (kind, name) in the universe's data folder, minted and
// registered when it has none; one already minted is kept, so programs still
// connected across a hub restart keep theirs. The hub is the one caller: it
// mints its own, the GUI's and the CLI's at start and a sidecar's before it
// spawns it. tenant is the hub's: a key's tenant is its account's.
func Ensure(dataDir, kind, name, tenant string) (*Credential, error) {
	if kind == "" || name == "" || tenant == "" {
		return nil, fmt.Errorf("busauth: a key needs a kind, a name and a tenant (got %q, %q, %q)", kind, name, tenant)
	}
	mintMu.Lock()
	defer mintMu.Unlock()
	dir := KeysDir(dataDir)
	path := CredentialPath(dir, kind, name)
	want := Caller{Kind: kind, Name: name, Tenant: tenant}
	cred, err := LoadCredential(path)
	if err != nil {
		return nil, err
	}
	if cred != nil && cred.Caller != want {
		cred = nil // minted for another tenant: minted again
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("busauth: create %s: %w", dir, err)
	}
	if cred == nil {
		pair, err := nkeys.CreateUser()
		if err != nil {
			return nil, fmt.Errorf("busauth: mint a key: %w", err)
		}
		seed, err := pair.Seed()
		if err != nil {
			return nil, err
		}
		raw, err := json.Marshal(credentialFile{Caller: want, Seed: string(seed)})
		if err != nil {
			return nil, err
		}
		if err := writeAtomic(path, raw, 0o600); err != nil {
			return nil, err
		}
		pub, _ := pair.PublicKey()
		cred = &Credential{Caller: want, dir: dir, public: pub, pair: pair}
	}
	regPath := filepath.Join(dir, registryFile)
	keys, err := readMintRegistry(regPath)
	if errors.Is(err, errUndecodable) {
		keys = map[string]Caller{} // a registry that does not decode is written again
	} else if err != nil {
		// A read that fails is never taken for an empty registry: writing
		// it back would drop every other program's key.
		return nil, err
	}
	if keys[cred.public] != want || hasOther(keys, cred.public, want) {
		for pub, c := range keys {
			if c.Kind == want.Kind && c.Name == want.Name && pub != cred.public {
				delete(keys, pub) // the key it replaces no longer connects
			}
		}
		keys[cred.public] = want
		raw, err := json.MarshalIndent(keys, "", "  ")
		if err != nil {
			return nil, err
		}
		if err := writeAtomic(regPath, raw, 0o600); err != nil {
			return nil, err
		}
		OpenRegistry(dir).forget()
	}
	return cred, nil
}

// mintReadFile reads the registry for a mint (a test injects failures).
var mintReadFile = os.ReadFile

// mintReadTries bounds how often a mint reads a registry file whose read
// fails (a lock held a moment by a scanner or a rename) before it gives up.
const mintReadTries = 10

// readMintRegistry reads the registry for a mint, again on a read that fails
// for a moment; a file that does not decode is not read again.
func readMintRegistry(path string) (map[string]Caller, error) {
	var err error
	for try := 0; try < mintReadTries; try++ {
		var keys map[string]Caller
		if keys, err = readRegistryWith(mintReadFile, path); err == nil || errors.Is(err, errUndecodable) {
			return keys, err
		}
		time.Sleep(20 * time.Millisecond)
	}
	return nil, fmt.Errorf("busauth: read the registry %s: %w", path, err)
}

// hasOther reports another key of want's kind and name in keys.
func hasOther(keys map[string]Caller, pub string, want Caller) bool {
	for p, c := range keys {
		if c.Kind == want.Kind && c.Name == want.Name && p != pub {
			return true
		}
	}
	return false
}

func writeAtomic(path string, raw []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("busauth: write %s: %w", path, err)
	}
	name := tmp.Name()
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		os.Remove(name)
		return fmt.Errorf("busauth: write %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	_ = os.Chmod(name, mode)
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return fmt.Errorf("busauth: write %s: %w", path, err)
	}
	return nil
}
