// SPDX-License-Identifier: Apache-2.0

package controlplane

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
)

// InstallationsEnv overrides where the registry of installations is kept
// (tests, sandboxes).
const InstallationsEnv = cenv.InstancesEnv

// KnownInstallation is one installation that ran katyoti on this machine: the
// name it was recorded under and its folder.
type KnownInstallation struct {
	Name   string `yaml:"name"`
	Folder string `yaml:"folder"`
}

// InstallationsRegistryPath is where the registry of installations is kept:
// $KTW_INSTANCES, else <user config folder>/katyoti-instances.yaml - outside
// every installation, so it reads the same from inside any of them.
// A test process never resolves to the person's registry:
// with KTW_INSTANCES empty or naming the person's file, it gets its own.
func InstallationsRegistryPath() (string, error) {
	p := os.Getenv(InstallationsEnv)
	if p == "" {
		dir, err := os.UserConfigDir()
		if err != nil {
			return "", err
		}
		p = filepath.Join(dir, "katyoti-instances.yaml")
	}
	if testing.Testing() && personsRegistry != "" && strings.EqualFold(filepath.Clean(p), personsRegistry) {
		return cenv.TestInstancesRegistry(), nil
	}
	return p, nil
}

// personsRegistry is the person's registry of installations as the process
// started (before a test moves its config folder): what a test never writes.
var personsRegistry = func() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Clean(filepath.Join(dir, "katyoti-instances.yaml"))
}()

// KnownInstallations reads the registry of installations (missing: none).
// Its one reader: katyoti's instances and the import of earlier projects
// both ask here. An entry whose folder no longer exists is forgotten
// : it is left out, the registry is written without it and the
// log says so - never listed at 0 B nor refused as sharing a folder.
func KnownInstallations() ([]KnownInstallation, error) {
	list, err := readInstallations()
	if err != nil {
		return nil, err
	}
	kept, gone := pruneVanished(list)
	if len(gone) == 0 {
		return list, nil
	}
	for _, k := range gone {
		slog.Info("katyoti-instances.yaml: an installation whose folder is gone is forgotten", "name", k.Name, "folder", k.Folder)
	}
	if err := writeInstallations(kept); err != nil {
		slog.Warn("katyoti-instances.yaml: the vanished installations could not be removed from it", "error", err)
	}
	return kept, nil
}

// pruneVanished splits the registry into the entries whose folder is there
// and those whose folder is gone. A folder that cannot be told gone (no access,
// a drive or share not mounted) is kept: only a missing folder on a volume
// that is there is forgotten.
func pruneVanished(list []KnownInstallation) (kept, gone []KnownInstallation) {
	for _, k := range list {
		if cenv.Vanished(k.Folder) {
			gone = append(gone, k)
			continue
		}
		kept = append(kept, k)
	}
	return kept, gone
}

// readInstallations reads the registry as it is written.
func readInstallations() ([]KnownInstallation, error) {
	p, err := InstallationsRegistryPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var r struct {
		Instances []KnownInstallation `yaml:"instances"`
	}
	if err := config.DecodeBytes(data, marshal.YAML, &r); err != nil {
		return nil, fmt.Errorf("read %s: %w", p, err)
	}
	return r.Instances, nil
}
