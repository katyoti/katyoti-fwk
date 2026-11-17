// SPDX-License-Identifier: Apache-2.0

package core

import (
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// Filing and naming (phase 7c, concepts.md §5 "Filing & naming"): a new
// object gets a readable id in its type's scheme and a file whose name is
// readable in a listing. The FILE NAME IS NEVER IDENTITY — the id in the
// content is — so the slug is a convenience and nothing resolves through it.

// objectFileNameFor is the file name of a NEW object: the local id, a slug of
// the title when the id was minted, and the spelling the body asks for —
// `.cwm.md` for prose, `.cwm.yaml` otherwise.
func objectFileNameFor(module string, obj api.Object, slug string) string {
	name := strings.TrimPrefix(obj.ID, module+".")
	if slug != "" {
		name += "-" + slug
	}
	if obj.Body != "" {
		return name + api.ObjectFileSuffixMD
	}
	return name + ObjectFileSuffix
}

// slugOfMax is the longest slug a file name carries.
const slugOfMax = api.SlugMax

// slugOf turns a title into a filename slug - api.Slug, the one spelling a
// module's id shares.
func slugOf(title string) string { return api.Slug(title) }

// mintObjectID gives an object created with a title and no id its id:
// `<id_prefix>-<NNN>`, the next number after the highest the module already
// holds for that prefix — archived ids counted — zero-padded to three (in a
// git repository, the next in the minter's number block); or a UUIDv7 when the type
// declares no prefix — the runtime kinds created concurrently, where a
// sequence would need a lock.
func (s *Service) mintObjectID(module string, t api.ObjectType, declared bool) (string, error) {
	if !declared || t.IDPrefix == "" {
		u, err := uuid.NewV7()
		if err != nil {
			return "", fmt.Errorf("mint id: %w", err)
		}
		return api.QualifyObjectID(module, u.String()), nil
	}
	prefix := t.IDPrefix + "-"
	// In a git repository the number comes from the minter's block
	// (number_blocks.go).
	if nb := s.blocks(); nb.repo != nil {
		n, err := s.mintFromBlock(nb, module, prefix)
		if err != nil {
			return "", err
		}
		return api.QualifyObjectID(module, fmt.Sprintf("%s%03d", prefix, n)), nil
	}
	ix := s.index()
	_ = ix.ensureLoaded()
	max := 0
	ids := []string{}
	for _, o := range ix.list(api.ObjectListInput{Module: module}) {
		ids = append(ids, o.ID)
	}
	// An ENDED id is taken for good: minting it again would make every link
	// to the archived object point at a new one (ended.go).
	ids = append(ids, s.endedIDsOf(module)...)
	// An id a create in flight has reserved is taken too (write_locks.go).
	s.locks.reserved.Range(func(k, _ any) bool {
		ids = append(ids, k.(string))
		return true
	})
	for _, id := range ids {
		if !strings.HasPrefix(id, module+".") {
			continue
		}
		local := strings.TrimPrefix(id, module+".")
		if !strings.HasPrefix(local, prefix) {
			continue
		}
		digits := strings.SplitN(strings.TrimPrefix(local, prefix), "-", 2)[0]
		n := 0
		for _, c := range digits {
			if c < '0' || c > '9' {
				n = -1
				break
			}
			n = n*10 + int(c-'0')
		}
		if n > max {
			max = n
		}
	}
	return api.QualifyObjectID(module, fmt.Sprintf("%s%03d", prefix, max+1)), nil
}
