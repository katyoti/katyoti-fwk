// SPDX-License-Identifier: Apache-2.0

package core

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/internal/numberblock"
)

// NUMBER BLOCKS (decision). A project that is the top
// level of a git repository mints its readable numbers from blocks its minter
// reserved by a commit in that repository (numberblock): two clones, two PCs or
// one repository opened in two tenants never mint the same number. A project
// in no repository, or a service given no minter folder, mints as before: the
// next number after the highest the module holds.

// defaultBlockSize is the numbers one reservation takes.
const defaultBlockSize = 50

// maxNumberDigits bounds the numbers counted as readable: a dated id
// (TD-20261103...) is no readable number.
const maxNumberDigits = 6

// numberBlocks is the service's minting state, built on first use.
type numberBlocks struct {
	once   sync.Once
	mu     sync.Mutex
	repo   *numberblock.Repo // nil: mint as before
	minter numberblock.Minter
	dir    string
}

// blocks answers the minting state; its repo is nil when blocks are off.
func (s *Service) blocks() *numberBlocks {
	nb := &s.numberBlocks
	nb.once.Do(func() {
		if s.Config == nil || s.Config.MinterDir == "" || s.Config.ObjectsRoot() == "" {
			return
		}
		repo, ok := numberblock.Open(s.Config.ObjectsRoot())
		if !ok {
			return
		}
		m, err := numberblock.LoadMinter(s.Config.MinterDir, "")
		if err != nil {
			slog.Warn("cwm: no minter - numbers are minted as in a project out of git", "error", err)
			return
		}
		nb.repo, nb.minter, nb.dir = repo, m, s.Config.MinterDir
	})
	return nb
}

// minterID is the minter a created object records, "" when blocks are off.
func (s *Service) minterID() string {
	if nb := s.blocks(); nb.repo != nil {
		return nb.minter.ID
	}
	return ""
}

// numberOf reads the number of a local id `<prefix>-<NNN>[-...]`.
func numberOf(local, prefix string) (int, bool) {
	if !strings.HasPrefix(local, prefix) {
		return 0, false
	}
	digits := strings.SplitN(strings.TrimPrefix(local, prefix), "-", 2)[0]
	if digits == "" || len(digits) > maxNumberDigits {
		return 0, false
	}
	n := 0
	for _, c := range digits {
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int(c-'0')
	}
	return n, true
}

// readableNumber reads the number of any readable id: `<module>.<PREFIX>-<NNN>`.
func readableNumber(id string) (int, bool) {
	local := id
	if i := strings.LastIndexByte(id, '.'); i >= 0 {
		local = id[i+1:]
	}
	i := strings.IndexByte(local, '-')
	if i <= 0 {
		return 0, false
	}
	for _, c := range local[:i] {
		if c < 'A' || c > 'Z' {
			return 0, false
		}
	}
	return numberOf(local, local[:i+1])
}

// takenIDs is every id a mint must not give again: the index's, the ended ones
// of the modules given (nil: every module) and those a create in flight holds.
func (s *Service) takenIDs(modules []string) []string {
	ix := s.index()
	_ = ix.ensureLoaded()
	var ids []string
	seen := map[string]bool{}
	for _, o := range ix.list(api.ObjectListInput{}) {
		ids = append(ids, o.ID)
		if modules == nil {
			if m := endedModuleOf(o.ID); m != "" {
				seen[m] = true
			}
		}
	}
	if modules == nil {
		if entries, err := os.ReadDir(filepath.Join(s.Config.ObjectsRoot(), s.modulesDirName())); err == nil {
			for _, e := range entries {
				if e.IsDir() {
					seen[e.Name()] = true
				}
			}
		}
		for m := range seen {
			modules = append(modules, m)
		}
	}
	for _, m := range modules {
		ids = append(ids, s.endedIDsOf(m)...)
	}
	s.locks.reserved.Range(func(k, _ any) bool {
		ids = append(ids, k.(string))
		return true
	})
	return ids
}

// highestReadable is the highest readable number the repository uses, of any
// prefix and module.
func (s *Service) highestReadable() int {
	h := 0
	for _, id := range s.takenIDs(nil) {
		if n, ok := readableNumber(id); ok && n > h {
			h = n
		}
	}
	return h
}

// mintFromBlock answers the next number for (module, prefix) in the minter's
// open block, reserving a new block when there is none or it is spent. The
// open block is the minter's latest in the ledger: the cache only says whether
// it still waits for its push, so a cache rolled back never brings an old
// block back.
func (s *Service) mintFromBlock(nb *numberBlocks, module, prefix string) (int, error) {
	nb.mu.Lock()
	defer nb.mu.Unlock()
	cache := numberblock.LoadCache(nb.dir)
	key := nb.repo.Dir
	ledger, _, err := nb.repo.ReadLedger()
	if err != nil {
		return 0, err
	}
	open, ok := ledger.LatestOf(nb.minter.ID)
	if ok {
		if c, cached := cache.Open[key]; cached && c.Provisional && c.Start == open.Start {
			confirmed, err := nb.repo.Confirm(open)
			switch {
			case errors.Is(err, numberblock.ErrBlockLost):
				slog.Warn("cwm: a provisional number block was refused by the remote; a new one is reserved",
					"start", open.Start, "end", open.End)
				ok = false
				if ledger, _, err = nb.repo.ReadLedger(); err != nil {
					return 0, err
				}
				if again, found := ledger.LatestOf(nb.minter.ID); found {
					open, ok = again, true
				}
			case err != nil:
				return 0, err
			case confirmed:
				open.Provisional = false
				cache.Open[key] = open
				_ = cache.Save(nb.dir)
			default:
				open.Provisional = true
			}
		}
	}
	next := func(b numberblock.Block) int {
		max := b.Start - 1
		for _, id := range s.takenIDs([]string{module}) {
			if !strings.HasPrefix(id, module+".") {
				continue
			}
			if n, ok := numberOf(strings.TrimPrefix(id, module+"."), prefix); ok && b.Holds(n) && n > max {
				max = n
			}
		}
		return max + 1
	}
	if ok {
		if n := next(open); open.Holds(n) {
			return n, nil
		}
	}
	size := s.Config.NumberBlockSize
	if size <= 0 {
		size = defaultBlockSize
	}
	b, err := nb.repo.Reserve(nb.minter, size, s.highestReadable())
	if err != nil {
		return 0, fmt.Errorf("mint id: %w", err)
	}
	cache.Open[key] = b
	if err := cache.Save(nb.dir); err != nil {
		slog.Warn("cwm: the number-block cache could not be written", "error", err)
	}
	return next(b), nil
}

// renumberOf names, of two objects carrying one id, the one to renumber: the
// one whose minter does not own the block holding the number - "" when the
// ledger cannot tell (blocks off, a number in no block, both or neither owner).
func (s *Service) renumberOf(a, b api.Object) string {
	nb := s.blocks()
	if nb.repo == nil {
		return ""
	}
	n, ok := readableNumber(a.ID)
	if !ok {
		return ""
	}
	ledger, _, err := nb.repo.ReadLedger()
	if err != nil {
		return ""
	}
	owner, found := ledger.OwnerOf(n)
	if !found {
		return ""
	}
	aOwns, bOwns := a.Minter == owner.Minter, b.Minter == owner.Minter
	switch {
	case aOwns && !bOwns:
		return "b"
	case bOwns && !aOwns:
		return "a"
	}
	return ""
}
