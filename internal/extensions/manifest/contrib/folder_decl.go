// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/config"
	contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
)

// FolderDecl is a served folder's FolderManifestFile as the pack reader reads
// it: who the folder is and where its kinds sit. The generic host adopts its
// identity; the hub reads a folder member with it. The `config:` block of the
// same file is sidecar.FolderConfig's.
type FolderDecl struct {
	// Name is the provider the folder publishes under.
	Name string `yaml:"name"`
	// Version is the CONTENT's version, not the host binary's: what
	// `describe` reports, stamped when the folder is published.
	Version     string `yaml:"version,omitempty"`
	DisplayName string `yaml:"displayName,omitempty"`
	Icon        string `yaml:"icon,omitempty"`
	Description string `yaml:"description,omitempty"`
	// Dirs remaps a kind to a subdirectory (relative to the folder,
	// forward-slash). A key is a kind's folder (`skills`, `data-types`) or a
	// kind's name, its value the folder to read that kind from instead, ""
	// switching it off even when its conventional folder is present. A key
	// naming a kind the framework has never heard of maps the folder its
	// entries sit in: the declaration ships in the pack's
	// `contribution-types/` folder (or in another member). `cwm` and
	// `scripts` (HostFolders) are folders, not kinds.
	Dirs map[string]string `yaml:"dirs,omitempty"`
}

// HostFolders are the folders of a served folder that are no kind's: its
// CWM pack and its scripts, each read by its own reader.
var HostFolders = []string{"cwm", "scripts"}

// ReadFolderDecl reads the folder's FolderManifestFile through ldr. A folder
// without one declares nothing: the zero FolderDecl and found false.
func ReadFolderDecl(ldr loader.Loader) (d FolderDecl, found bool, err error) {
	raw, rerr := ldr.ReadFile(FolderManifestFile)
	if rerr != nil || raw == nil {
		return FolderDecl{}, false, nil
	}
	if err := config.DecodeBytes(raw, marshal.YAML, &d); err != nil {
		return FolderDecl{}, true, fmt.Errorf("%s: %w", FolderManifestFile, err)
	}
	return d, true, nil
}

// Remap is the declaration's `dirs:` as the pack reader's remap (RemapOfDirs),
// the HostFolders left to their readers; bad hears of a key that is no kind.
func (d FolderDecl) Remap(kinds contribapi.Kinds, bad func(key string, err error)) map[string]string {
	return RemapOfDirs(d.Dirs, kinds, bad, HostFolders...)
}

// HostDir is the subdirectory (relative, forward-slash) a host folder (cwm,
// scripts) is read from in the folder at root, or "" to skip it: the `dirs:`
// entry when declared ("" disables), else the conventional folder when it
// exists on disk — or when the pack records content under it (recorded):
// bulk content moved to the object store leaves no file, and a folder whose
// only content was bulk no directory either; skipping it would ship the
// pack silently short.
func (d FolderDecl) HostDir(root, typ string, recorded contribapi.LayerManifest) string {
	dir, declared := d.Dirs[typ]
	if !declared {
		dir = typ
	}
	if dir == "" {
		return ""
	}
	if fi, err := os.Stat(filepath.Join(root, filepath.FromSlash(dir))); err == nil && fi.IsDir() {
		return dir
	}
	prefix := strings.TrimSuffix(dir, "/") + "/"
	for rel := range recorded.Layers {
		if strings.HasPrefix(rel, prefix) {
			return dir
		}
	}
	return ""
}
