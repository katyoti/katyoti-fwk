// SPDX-License-Identifier: Apache-2.0

package authoring

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"time"

	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/marketplace/internal/signing"
	"gopkg.in/yaml.v3"
)

// provenance.go — producing origin assertions at the moment of distribution.
//
// Packaging is where authorship becomes distribution: it turns a folder someone
// has been editing into a versioned artifact that will be installed and run
// somewhere else. That is the moment a publisher's intent exists to be
// expressed, and the moment content stops being local — so it is where the
// assertion is made. Content an author serves from their own folder is not a
// distribution event and is deliberately left unsigned.
//
// The assertions travel in a file BESIDE the content rather than inside each
// content file, because a files-only pack's contributions are not files: a host
// DERIVES them from the folder. There is nothing to stamp until they are
// derived, so they are stamped at that moment, from this.

// ProvenanceFile is the well-known name of the assertions file inside a pack.
// Fixed, like the pack's own manifest, so a renamed host still finds it.
const ProvenanceFile = "ktw-provenance.yaml"

// provenanceDoc is the on-disk shape.
type provenanceDoc struct {
	Version int `yaml:"version"`
	// Publisher is the claim, repeated once rather than per entry.
	Publisher string `yaml:"publisher,omitempty"`
	// Assertions maps a contribution URI to its signature over that
	// contribution's identity digest.
	Assertions []provenanceEntry `yaml:"assertions"`
}

// provenanceEntry is one contribution's assertion. A list rather than a map so
// the file has a stable order: an unchanged pack rebuilt must produce identical
// bytes, or every rebuild looks like a change.
type provenanceEntry struct {
	URI       string `yaml:"uri"`
	KeyID     string `yaml:"keyId,omitempty"`
	Signature string `yaml:"signature"`
}

// WriteAssertions signs every contribution's identity and writes the result
// into `folder`, to be packed with it.
//
// Returns the number of assertions written. Writing nothing is not an error:
// packaging without a key is a supported way to publish, because an unsigned
// contribution is valid and usable.
func WriteAssertions(folder string, cs []contribapi.Contribution, sk signing.SecretKey, publisher string) (int, error) {
	if len(cs) == 0 {
		return 0, nil
	}
	as := signing.SignAll(cs, sk, publisher)
	doc := provenanceDoc{Version: 1, Publisher: publisher}
	uris := make([]string, 0, len(as))
	for uri := range as {
		uris = append(uris, uri)
	}
	sort.Strings(uris)
	for _, uri := range uris {
		p := as[uri]
		doc.Assertions = append(doc.Assertions, provenanceEntry{
			URI: uri, KeyID: p.KeyID, Signature: p.Signature,
		})
	}
	out, err := yaml.Marshal(&doc)
	if err != nil {
		return 0, fmt.Errorf("encode provenance: %w", err)
	}
	dest := filepath.Join(folder, ProvenanceFile)
	if err := os.WriteFile(dest, out, 0o644); err != nil {
		return 0, fmt.Errorf("write %s: %w", dest, err)
	}
	return len(doc.Assertions), nil
}

// ReadAssertions loads a pack's assertions, keyed by contribution URI.
//
// A pack with no assertions file returns an empty map and no error: unsigned is
// a normal state, not a failure. Refusing to serve unsigned content would be an
// enforcement policy, and enforcement is not this module's to make.
func ReadAssertions(folder string) (signing.Assertions, error) {
	b, err := os.ReadFile(filepath.Join(folder, ProvenanceFile))
	if err != nil {
		if os.IsNotExist(err) {
			return signing.Assertions{}, nil
		}
		return nil, fmt.Errorf("read %s: %w", ProvenanceFile, err)
	}
	var doc provenanceDoc
	if err := yaml.Unmarshal(b, &doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", ProvenanceFile, err)
	}
	out := make(signing.Assertions, len(doc.Assertions))
	for _, e := range doc.Assertions {
		out[e.URI] = contribapi.Provenance{
			Signature: e.Signature, KeyID: e.KeyID, Publisher: doc.Publisher,
		}
	}
	return out, nil
}

// StampAssertions attaches each contribution's assertion, where one exists.
//
// Attaching is all this does: it does not verify. A host serving a pack has the
// publisher's signature but not necessarily the publisher's key, and whether an
// assertion is trusted is the consumer's decision, made with the keys the
// consumer holds. Carrying the claim forward is what makes that decision
// possible later.
func StampAssertions(cs []contribapi.Contribution, as signing.Assertions) []contribapi.Contribution {
	if len(as) == 0 {
		return cs
	}
	for i := range cs {
		if p, ok := as[cs[i].ResolvedURI()]; ok {
			assertion := p
			cs[i].Provenance = &assertion
		}
	}
	return cs
}

// ProbeContributions asks a host binary what a folder publishes.
//
// Packaging cannot read a files-only pack's contributions off disk, because it
// does not have any: a host DERIVES them from the folder's layout. So the
// packaging step asks the same question, of the same binary, in the same way
// the aggregator does — one subprocess, no bus, no universe.
//
// The content root travels in the environment rather than as a flag, for the
// same reason it does everywhere else: it is declared, not recovered.
func ProbeContributions(hostBinary, folder string) ([]contribapi.Contribution, error) {
	cs, _, err := ProbeFolder(hostBinary, folder)
	return cs, err
}

// ProbeFolder asks the host what a folder publishes, AND what it could not.
//
// The second answer is the one packaging needs and nothing else does: a file
// the host could not read as content produces no contribution, so a check that
// reasons about contributions cannot see it and the pack ships short. The host
// records the fact; this is where it crosses back.
func ProbeFolder(hostBinary, folder string) ([]contribapi.Contribution, []contribapi.Drop, error) {
	if hostBinary == "" || folder == "" {
		return nil, nil, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, hostBinary, "sc", "resources", "manifest")
	cmd.Env = append(os.Environ(), "KTW_CONTENT_ROOT="+folder)
	out, err := cmd.Output()
	if err != nil {
		return nil, nil, fmt.Errorf("probe %s for %s: %w", hostBinary, folder, err)
	}
	var m struct {
		Contributions []contribapi.Contribution `json:"contributions"`
		Drops         []contribapi.Drop         `json:"drops"`
	}
	if err := json.Unmarshal(out, &m); err != nil {
		return nil, nil, fmt.Errorf("decode probe of %s: %w", folder, err)
	}
	return m.Contributions, m.Drops, nil
}

// SignFolder probes a folder and writes its assertions, ready to be packed.
//
// A no-op without a key: publishing unsigned is supported, so the absence of a
// key is a choice rather than an error. A probe failure IS reported — it means
// packaging could not see what it is about to ship, which is worth knowing
// whether or not anything is being signed.
func SignFolder(hostBinary, folder, secretKeyPath, publisher string) (int, error) {
	if secretKeyPath == "" {
		return 0, nil
	}
	raw, err := os.ReadFile(secretKeyPath)
	if err != nil {
		return 0, fmt.Errorf("read signing key %s: %w", secretKeyPath, err)
	}
	sk, err := signing.ParseSecretKey(raw)
	if err != nil {
		return 0, fmt.Errorf("parse signing key %s: %w", secretKeyPath, err)
	}
	cs, err := ProbeContributions(hostBinary, folder)
	if err != nil {
		return 0, err
	}
	return WriteAssertions(folder, cs, sk, publisher)
}
