// SPDX-License-Identifier: Apache-2.0

package core

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/extensions/scenario/api"
)

// THE INSTALL STAMP — what a case was installed from, so a stale install is
// never reused: the case's definition of what install sets up
// (its universes and its project) and, per binary copied into its bin folder,
// the build it was copied from (its size and time in the source folder). A
// start, a run and a replay compare it with today's: another definition is
// refused (install it again), another build is refused (refresh its
// binaries). It lives beside the case's runtime, never in a checkpoint: a
// restore never brings back an older stamp.

// stampFile is the stamp, in the case's runtime folder.
const stampFile = "install-stamp.json"

type installStamp struct {
	Case string            `json:"case"`
	Bin  map[string]string `json:"bin"`
}

// caseFingerprint is the hash of what install reads from the case: its name,
// its universes, its project and its other repositories. Its stages, its user and its clean-machine
// part are read when it runs, so changing them needs no new install.
func (e *Engine) caseFingerprint() string {
	// Repositories only when the case has some: a case without them keeps
	// the fingerprint its install was stamped with.
	raw, _ := json.Marshal(struct {
		Name         string
		Universe     any
		Others       any
		Project      any
		Repositories []api.CaseRepository `json:",omitempty"`
	}{e.Case.Name, e.Case.Universe, e.Case.OtherUniverses, e.Case.Project, e.Case.Repositories})
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// binFingerprint is one binary's build as its source folder holds it; ""
// when it is not there.
func binFingerprint(path string) string {
	fi, err := os.Stat(path)
	if err != nil || fi.IsDir() {
		return ""
	}
	return fmt.Sprintf("%d:%d", fi.Size(), fi.ModTime().UnixNano())
}

// copiedBinaries are the binaries the case's bin folder holds copies of.
func (e *Engine) copiedBinaries() []string {
	out := e.caseBinaries()
	for _, b := range []string{guiBinaryName, universBinary} {
		if fileExists(filepath.Join(e.RT.Bin(), exe(b))) {
			out = append(out, b)
		}
	}
	return out
}

// binStamp is the build of each copied binary in src.
func (e *Engine) binStamp(src string) map[string]string {
	out := map[string]string{}
	for _, b := range e.copiedBinaries() {
		if fp := binFingerprint(filepath.Join(src, exe(b))); fp != "" {
			out[b] = fp
		}
	}
	return out
}

func (e *Engine) loadStamp() (installStamp, bool) {
	var s installStamp
	raw, err := os.ReadFile(filepath.Join(e.RT.Dir, stampFile))
	if err != nil || json.Unmarshal(raw, &s) != nil {
		return s, false
	}
	return s, true
}

func (e *Engine) saveStamp(s installStamp) error {
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(e.RT.Dir, stampFile), raw, 0o644)
}

// stampInstall records what the case is being installed from.
func (e *Engine) stampInstall() error {
	return e.saveStamp(installStamp{Case: e.caseFingerprint(), Bin: e.binStamp(e.BinSrc)})
}

// stampBinaries records the build its binaries were just copied from,
// keeping the definition it was installed from.
func (e *Engine) stampBinaries() error {
	s, ok := e.loadStamp()
	if !ok {
		return nil // never stamped: still refused until installed again
	}
	s.Bin = e.binStamp(e.BinSrc)
	return e.saveStamp(s)
}

// CheckFresh refuses a stale install: one with no stamp (made before stamps),
// one made from another definition of the case, or one whose binaries came
// from a build its source folder no longer holds. An installed universe a
// clean machine's case runs on (Universe set) is not the case's install.
func (e *Engine) CheckFresh() error {
	if e.Universe != "" {
		return nil
	}
	name := e.Case.Name
	s, ok := e.loadStamp()
	if !ok {
		return fmt.Errorf("case %s was installed with no install stamp (by an older build): install it again (scenario install %s --force)", name, name)
	}
	if s.Case != e.caseFingerprint() {
		return fmt.Errorf("case %s's definition (its universes or its project) changed since it was installed: install it again (scenario install %s --force)", name, name)
	}
	src := BinSource(e.RT)
	var changed []string
	for b, fp := range s.Bin {
		if now := binFingerprint(filepath.Join(src, exe(b))); now != "" && now != fp {
			changed = append(changed, b)
		}
	}
	if len(changed) > 0 {
		slices.Sort(changed)
		return fmt.Errorf("case %s was installed from another build of %s (%s changed since): refresh its binaries (scenario run %s --refresh-bin, or scenario refresh-bin %s)",
			name, src, strings.Join(changed, ", "), name, name)
	}
	return nil
}
