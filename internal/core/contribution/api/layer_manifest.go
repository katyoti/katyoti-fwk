// SPDX-License-Identifier: Apache-2.0

package contribapi

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// layer_manifest.go — what a pack says about content it no longer carries.
//
// A layer's descriptor is normally DERIVED: stat the file, hash it, read the
// band off the size. That works for as long as the file is there.
//
// Packaging writes a bulk layer to the object store and leaves the file out of
// the pack — that is the entire point, so a 40 GB dataset makes a pack three
// lines longer instead of 40 GB bigger. From that moment nothing can derive the
// descriptor, because the thing it was derived from is gone. So it is RECORDED,
// at the one moment the content was still present to measure.
//
// Same reasoning as `ktw-provenance.yaml` (c16), arriving from the other
// direction: that file exists because there is nothing to stamp until the
// contributions exist, this one because there is nothing to measure once the
// content has left.

// LayerManifestFile is the recorded descriptors, at a pack's root.
const LayerManifestFile = "ktw-layers.yaml"

// LayerManifest maps a file's path within the pack — slash-separated, relative
// to the pack root — to what its content is.
//
// Keyed by PATH rather than by URI on purpose. The provider that needs this is
// looking at a file it just failed to stat; it has not derived an ID yet, and
// the ID derivation differs per provider. The path is what both sides hold.
type LayerManifest struct {
	APIVersion int                 `yaml:"apiVersion"`
	Layers     map[string]LayerRef `yaml:"layers"`
}

// ReadLayerManifest loads a pack's recorded descriptors.
//
// An absent file is not an error: most packs have no bulk content and carry no
// manifest, and treating that as a failure would make the common case pay for
// the rare one.
func ReadLayerManifest(packRoot string) (LayerManifest, error) {
	raw, err := os.ReadFile(filepath.Join(packRoot, LayerManifestFile))
	if os.IsNotExist(err) {
		return LayerManifest{}, nil
	}
	if err != nil {
		return LayerManifest{}, fmt.Errorf("read %s: %w", LayerManifestFile, err)
	}
	return ParseLayerManifest(raw)
}

// ParseLayerManifest decodes recorded descriptors from bytes.
//
// Separate from ReadLayerManifest because the two readers reach the file
// differently: packaging holds a filesystem path, while a provider reaches it
// through its own Loader — which may be an embedded FS with no path at all.
// Empty input is an empty manifest, for the same reason an absent file is.
func ParseLayerManifest(raw []byte) (LayerManifest, error) {
	if len(raw) == 0 {
		return LayerManifest{}, nil
	}
	var m LayerManifest
	if err := yaml.Unmarshal(raw, &m); err != nil {
		return LayerManifest{}, fmt.Errorf("parse %s: %w", LayerManifestFile, err)
	}
	return m, nil
}

// WriteLayerManifest records descriptors at a pack's root, replacing any
// previous file. Writing an empty set removes the file rather than leaving an
// empty one, so "this pack has no bulk content" has one representation.
//
// Deterministic: yaml.v3 emits map keys in sorted order, so re-recording
// unchanged content produces identical bytes. A packaging step whose output
// churned would make every rebuild look like a change to the content digest
// that drives staleness.
func WriteLayerManifest(packRoot string, layers map[string]LayerRef) error {
	path := filepath.Join(packRoot, LayerManifestFile)
	if len(layers) == 0 {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove %s: %w", LayerManifestFile, err)
		}
		return nil
	}
	out, err := yaml.Marshal(LayerManifest{APIVersion: 1, Layers: layers})
	if err != nil {
		return fmt.Errorf("marshal %s: %w", LayerManifestFile, err)
	}
	header := "# Recorded by packaging. Each entry is content held in the object\n" +
		"# store under its hash rather than carried in this pack.\n"
	if err := os.WriteFile(path, append([]byte(header), out...), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", LayerManifestFile, err)
	}
	return nil
}
