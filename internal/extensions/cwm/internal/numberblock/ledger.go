// SPDX-License-Identifier: Apache-2.0

package numberblock

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/katyoti/katyoti-fwk/internal/core/atomicio"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
)

// Block is one range of numbers a minter reserved: every module and every
// prefix of the repository may take its numbers from it.
type Block struct {
	Start  int    `yaml:"start"`
	End    int    `yaml:"end"`
	Minter string `yaml:"minter"`
	Person string `yaml:"person,omitempty"`
	At     string `yaml:"at,omitempty"`
	// Provisional marks, in the minter's cache only, a block reserved while
	// the remote could not be reached: confirmed at its next push.
	Provisional bool `yaml:"provisional,omitempty"`
}

// Holds says whether n lies in the block.
func (b Block) Holds(n int) bool { return n >= b.Start && n <= b.End }

// Ledger is the blocks reserved in one repository, in the order reserved.
type Ledger struct {
	Blocks []Block `yaml:"blocks"`
}

// HighestEnd is the highest number any block reserves (0 when none).
func (l Ledger) HighestEnd() int {
	h := 0
	for _, b := range l.Blocks {
		if b.End > h {
			h = b.End
		}
	}
	return h
}

// LatestOf is the minter's block reserved last.
func (l Ledger) LatestOf(minter string) (Block, bool) {
	var out Block
	found := false
	for _, b := range l.Blocks {
		if b.Minter == minter && (!found || b.Start > out.Start) {
			out, found = b, true
		}
	}
	return out, found
}

// OwnerOf is the block holding n, if any.
func (l Ledger) OwnerOf(n int) (Block, bool) {
	for _, b := range l.Blocks {
		if b.Holds(n) {
			return b, true
		}
	}
	return Block{}, false
}

func (l Ledger) encode() ([]byte, error) {
	sort.SliceStable(l.Blocks, func(i, j int) bool { return l.Blocks[i].Start < l.Blocks[j].Start })
	for i := range l.Blocks {
		l.Blocks[i].Provisional = false // a flag of the cache, never of the ledger
	}
	return marshal.Marshal(l, marshal.YAML)
}

func parseLedger(data []byte) (Ledger, error) {
	var l Ledger
	if err := config.DecodeBytes(data, marshal.YAML, &l); err != nil {
		return Ledger{}, fmt.Errorf("the number-block ledger does not read: %w", err)
	}
	return l, nil
}

// Minter is who reserves blocks: one per (tenant, installation), its id a
// UUIDv7 minted once. One person on two PCs, or a repository opened in two
// tenants, are two minters.
type Minter struct {
	ID     string `yaml:"id"`
	Person string `yaml:"person,omitempty"`
}

// MinterFile and CacheFile are the minter's files in its folder
// (TenantDataDir()/cwm).
const (
	MinterFile = "minter.yaml"
	CacheFile  = "number-block.yaml"
)

// LoadMinter reads the minter of dir, minting it on first use.
func LoadMinter(dir, person string) (Minter, error) {
	p := filepath.Join(dir, MinterFile)
	if data, err := os.ReadFile(p); err == nil {
		var m Minter
		if err := config.DecodeBytes(data, marshal.YAML, &m); err != nil || m.ID == "" {
			return Minter{}, fmt.Errorf("the minter %s does not read: %v", p, err)
		}
		return m, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return Minter{}, err
	}
	u, err := uuid.NewV7()
	if err != nil {
		return Minter{}, err
	}
	m := Minter{ID: u.String(), Person: person}
	data, err := marshal.Marshal(m, marshal.YAML)
	if err != nil {
		return Minter{}, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Minter{}, err
	}
	if err := atomicio.WriteAtomic(p, data, 0o644); err != nil {
		return Minter{}, err
	}
	return m, nil
}

// Cache is the minter's open block per repository (by its top level). It is a
// cache: the ledger and the ids already used win over it.
type Cache struct {
	Open map[string]Block `yaml:"open"`
}

// LoadCache reads the cache of dir (empty when absent or unreadable).
func LoadCache(dir string) Cache {
	c := Cache{Open: map[string]Block{}}
	data, err := os.ReadFile(filepath.Join(dir, CacheFile))
	if err != nil {
		return c
	}
	_ = config.DecodeBytes(data, marshal.YAML, &c)
	if c.Open == nil {
		c.Open = map[string]Block{}
	}
	return c
}

// Save writes the cache of dir.
func (c Cache) Save(dir string) error {
	data, err := marshal.Marshal(c, marshal.YAML)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return atomicio.WriteAtomic(filepath.Join(dir, CacheFile), data, 0o644)
}

// MaxRetries bounds the reservations a rejected push retries.
const MaxRetries = 8

// Reserve reserves the next block of size numbers for m: above every block the
// ledger holds and above floor (the highest number the repository uses). With
// a remote the remote's ledger is fetched first and the new one pushed; a push
// another reservation beat is retried on the remote's ledger with the next
// range. A remote that cannot be reached gives a provisional block, recorded
// locally and confirmed by Confirm.
func (r *Repo) Reserve(m Minter, size, floor int) (Block, error) {
	if size <= 0 {
		size = 50
	}
	r.ensureRefspec()
	var last string
	for attempt := 0; attempt < MaxRetries; attempt++ {
		online := r.Remote != "" && r.fetch()
		ledger, tip, err := r.ReadLedger()
		if err != nil {
			return Block{}, err
		}
		start := ledger.HighestEnd()
		if floor > start {
			start = floor
		}
		b := Block{Start: start + 1, End: start + size, Minter: m.ID, Person: m.Person, At: time.Now().UTC().Format(time.RFC3339)}
		next := Ledger{Blocks: append(append([]Block{}, ledger.Blocks...), b)}
		commit, err := r.commit(next, tip, fmt.Sprintf("number block %d-%d for minter %s", b.Start, b.End, m.ID))
		if err != nil {
			return Block{}, err
		}
		if !r.casRef(commit, tip) {
			last = "the local ledger moved under the reservation"
			continue
		}
		if r.Remote == "" {
			return b, nil
		}
		if !online {
			b.Provisional = true
			return b, nil
		}
		if r.beforePush != nil {
			r.beforePush()
		}
		switch res, msg := r.push(); res {
		case pushed:
			return b, nil
		case pushFailed:
			b.Provisional = true
			return b, nil
		default:
			last = msg
		}
	}
	return Block{}, fmt.Errorf("no number block could be reserved on remote %q after %d tries: %s", r.Remote, MaxRetries, last)
}

// Confirm pushes a provisional block: true when the remote took it (or there
// is no remote), false with nothing changed when the remote still cannot be
// reached. A remote holding another ledger refuses it: the block is dropped
// from the local ledger (the remote's is fetched) and ErrBlockLost answered -
// reserve a new one; the numbers already minted from it are the scan's to name.
func (r *Repo) Confirm(b Block) (bool, error) {
	if r.Remote == "" {
		return true, nil
	}
	switch res, _ := r.push(); res {
	case pushed:
		return true, nil
	case pushFailed:
		return false, nil
	}
	r.fetch()
	return false, ErrBlockLost
}

// ErrBlockLost is a provisional block the remote refused.
var ErrBlockLost = errors.New("the provisional number block was refused by the remote")
