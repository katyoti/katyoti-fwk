// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/appcore/controlplane"
	"github.com/katyoti/katyoti-fwk/internal/appcore/hubclient"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/cenv"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/admin"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/catalog"
)

// Service is the ktw-univers sidecar's implementation of api.OrchestratorService.
//
// It holds:
//   - registry: the cross-universe catalog (~/ktw-agents/universes.yaml)
//
// No active-universes map, no spawn logic, no daemon mode — those concerns
// live in ktw-gui's Go side (modules/gui/concepts.md §2 (the ActiveCore model)).
//
// Thread-safe (all CRUD ops take a write or read lock; the registry has its
// own internal lock too).
type Service struct {
	mu       sync.RWMutex
	registry *catalog.Registry
	// dialHub reaches a universe's running hub (hubdial.Dial when nil);
	// the tests give a fake one.
	dialHub hubDialer
}

// ServiceConfig is the bootstrap config for the ktw-univers service.
//
// v1 keeps a single field — RegistryPath — primarily as a test override.
// Production callers leave it empty to get catalog.DefaultRegistryPath().
type ServiceConfig struct {
	// RegistryPath overrides the universes.yaml location.
	// Empty = ~/.config/katyoti/universes.yaml (or %APPDATA%\katyoti\universes.yaml on Windows).
	// Test override via KTW_KATYOTI_ROOT env var, not this field.
	RegistryPath string `yaml:"registry_path,omitempty"`
}

// NewService is the publish-framework factory.
//
// Loads the registry once at startup. If the file is missing, an empty
// in-memory registry is returned (created on first Save).
//
// Auto-creates the admin universe if missing (idempotent + self-healing per
// the admin-universe ADR).
func NewService(ctx context.Context, cfg *ServiceConfig) (api.OrchestratorService, error) {
	if cfg == nil {
		cfg = &ServiceConfig{}
	}
	regPath := cfg.RegistryPath
	if regPath == "" {
		var err error
		regPath, err = catalog.DefaultRegistryPath()
		if err != nil {
			return nil, fmt.Errorf("resolve default registry path: %w", err)
		}
	}
	registry, err := catalog.LoadRegistry(regPath)
	if err != nil {
		return nil, fmt.Errorf("load registry: %w", err)
	}

	// Auto-create the admin universe if missing. Idempotent — re-running on
	// subsequent boots is a no-op. Self-healing — if either the registry
	// entry or the folder is missing, the absent half is recreated.
	// Owned by the admin package (shared with the katyoti launcher's pre-hub
	// bootstrap). See modules/univers/decisions/OD-001-where-does-a-fresh-install-manage-its-universes.cwm.md for full spec.
	if err := admin.Ensure(registry, admin.Home()); err != nil {
		return nil, fmt.Errorf("auto-create admin universe: %w", err)
	}

	return &Service{
		registry: registry,
	}, nil
}

// ============================================================================
// KTSP methods
// ============================================================================

func (s *Service) ListUniverses(ctx context.Context, in api.ListUniversesInput) (api.ListUniversesOutput, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entries := s.registry.List()
	out := api.ListUniversesOutput{Universes: make([]api.UniverseSummary, 0, len(entries))}
	for _, e := range entries {
		manifest, err := catalog.LoadManifestOrRepair(e.HomePath, e.Name)
		if err != nil {
			// Registry entry exists but manifest is broken — return the entry
			// with whatever info we can derive.
			out.Universes = append(out.Universes, api.UniverseSummary{
				Name:        e.Name,
				DisplayName: e.Name,
				Description: fmt.Sprintf("(error reading workspace.yaml: %v)", err),
				HomePath:    e.HomePath,
				Protected:   e.Protected,
			})
			continue
		}
		out.Universes = append(out.Universes, manifest.ToUniverseSummary(e.Name, e.HomePath, e.Protected))
	}
	return out, nil
}

func (s *Service) GetUniverse(ctx context.Context, in api.GetUniverseInput) (api.UniverseInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	entry, err := s.registry.Find(in.Name)
	if err != nil {
		return api.UniverseInfo{}, err
	}
	manifest, err := catalog.LoadManifestOrRepair(entry.HomePath, entry.Name)
	if err != nil {
		return api.UniverseInfo{}, err
	}
	return manifest.ToUniverseInfo(entry.Name, entry.HomePath, entry.Protected, nil), nil
}

func (s *Service) CreateUniverse(ctx context.Context, in api.CreateUniverseInput) (api.UniverseInfo, error) {
	if !validUniverseName(in.Name) {
		return api.UniverseInfo{}, fmt.Errorf("invalid universe name %q: must match ^[a-z0-9-]{1,64}$", in.Name)
	}
	// A project inside the installation is refused before the universe exists;
	// so is a new project of its own whose name a project holds already
	// : a folder named is attached as it is.
	if in.ProjectDir != "" {
		if err := cenv.CurrentInstallation().CheckProjectDir(in.ProjectDir); err != nil {
			return api.UniverseInfo{}, err
		}
	} else if in.WithProject {
		if _, _, refused := s.placeProject(in.Name); refused != "" {
			return api.UniverseInfo{}, fmt.Errorf("universe %q is not created: its project %s", in.Name, refused)
		}
	}
	// DEFAULT HERE, not in each CLI. Requiring it pushed the default into
	// every caller, and they disagreed: `ktw-univers create foo` produced
	// display "Foo" while `katyoti universe create foo` produced "foo" --
	// the same operation, two results, because the service that owns the
	// concept declined to answer.
	if in.DisplayName == "" {
		in.DisplayName = strings.ToUpper(in.Name[:1]) + in.Name[1:]
	}
	if in.HomePath == "" {
		// Default: <GlobalHomeDir>/universes/<name>/ — system universes live
		// under the katyoti global root.
		in.HomePath = filepath.Join(application.Env().GlobalHomeDir(), "universes", in.Name)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, err := s.registry.Find(in.Name); err == nil {
		return api.UniverseInfo{}, fmt.Errorf("universe %q already exists in registry", in.Name)
	}
	if _, statErr := os.Stat(in.HomePath); statErr == nil {
		return api.UniverseInfo{}, fmt.Errorf("home path %q already exists; choose a different path or delete the existing folder", in.HomePath)
	}

	if err := firstGuideOf(in); err != nil {
		return api.UniverseInfo{}, err
	}

	// Build the manifest from the input.
	sidecars := make([]api.SidecarSpec, 0, len(in.Sidecars))
	for _, sel := range in.Sidecars {
		sidecars = append(sidecars, api.SidecarSpec{
			Name:      sel.Name,
			Transport: "nats", // canonical bus-split convention (DO.3) — matches AddSidecar + wire.Into
			Binary:    sel.BinaryPath,
			Args:      []string{"sc", "ktsp", "nats"},
			ConfigDir: filepath.Join("sidecars", sel.Name),
		})
	}
	manifest := &catalog.Manifest{
		Version:     1,
		Name:        in.Name,
		DisplayName: in.DisplayName,
		Description: in.Description,
		Icon:        in.Icon,
		Color:       in.Color,
		Sidecars:    sidecars,
		FirstGuide:  in.FirstGuide,
		Tags:        tagsOf(in.Tags),
		NoWelcome:   in.NoWelcome,
	}

	// Create folder skeleton + write Univers.yaml (under .katyoti/config/).
	if err := os.MkdirAll(in.HomePath, 0o755); err != nil {
		return api.UniverseInfo{}, fmt.Errorf("create universe home: %w", err)
	}
	env := application.EnvOf(in.HomePath)
	configDir := env.ConfigDir()
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return api.UniverseInfo{}, fmt.Errorf("create config dir: %w", err)
	}
	if err := catalog.SaveManifest(in.HomePath, manifest); err != nil {
		return api.UniverseInfo{}, err
	}
	// Empty .env + Settings.yaml so the universe boots without errors.
	if err := catalog.EnsureSkeleton(in.HomePath); err != nil {
		return api.UniverseInfo{}, err
	}

	// Register in the catalog.
	if err := s.registry.Add(catalog.RegistryEntry{
		Name:     in.Name,
		HomePath: in.HomePath,
	}); err != nil {
		return api.UniverseInfo{}, fmt.Errorf("register universe: %w", err)
	}

	// Its project, when asked: without one a universe opens on an empty tree
	// (its objects live in the project it attaches). CWM gives a new folder its
	// project object, inbox and planification module the first time it opens.
	if in.WithProject || in.ProjectDir != "" {
		dir := in.ProjectDir
		if dir == "" {
			dir = defaultProjectDir(in.Name)
		}
		// A folder this creation makes is created on this PC
		// a folder named that exists already is attached as it is, unrecorded.
		// An attach that fails drops the record, as project create does: the
		// record never names a place no creation of this PC made.
		_, statErr := os.Stat(dir)
		made := os.IsNotExist(statErr)
		if made {
			_ = s.recordCreatedHere(dir)
		}
		if _, err := s.AttachProject(ctx, api.AttachProjectInput{Universe: in.Name, ProjectDir: dir}); err != nil {
			if made {
				s.forgetCreatedHere(dir)
			}
			return api.UniverseInfo{}, fmt.Errorf("universe %q is created, but its project %s is not attached: %w", in.Name, dir, err)
		}
	}

	notifyUniversesChanged(ctx, "created", in.Name)
	return manifest.ToUniverseInfo(in.Name, in.HomePath, false, nil), nil
}

// AddSidecar appends a sidecar to an existing universe's Univers.yaml.
// The new row follows the bus-split convention (transport nats, args
// "sc ktsp nats") — the convention every live universe row uses; the
// stdio default in CreateUniverse predates the bus split.
func (s *Service) AddSidecar(ctx context.Context, in api.AddSidecarInput) (api.UniverseInfo, error) {
	if in.Universe == "" {
		return api.UniverseInfo{}, fmt.Errorf("universe is required")
	}
	if in.Sidecar.Name == "" {
		return api.UniverseInfo{}, fmt.Errorf("sidecar.name is required")
	}
	if in.Sidecar.BinaryPath != "" {
		if _, err := os.Stat(in.Sidecar.BinaryPath); err != nil {
			return api.UniverseInfo{}, fmt.Errorf("sidecar binary %q not found: %w", in.Sidecar.BinaryPath, err)
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	entry, err := s.registry.Find(in.Universe)
	if err != nil {
		return api.UniverseInfo{}, fmt.Errorf("universe %q not in registry: %w", in.Universe, err)
	}
	manifest, err := catalog.LoadManifest(entry.HomePath)
	if err != nil {
		return api.UniverseInfo{}, fmt.Errorf("load Univers.yaml for %q: %w", in.Universe, err)
	}
	for _, sc := range manifest.Sidecars {
		if sc.Name == in.Sidecar.Name {
			return api.UniverseInfo{}, fmt.Errorf("sidecar %q already a member of universe %q", in.Sidecar.Name, in.Universe)
		}
	}
	manifest.Sidecars = append(manifest.Sidecars, api.SidecarSpec{
		Name:      in.Sidecar.Name,
		Transport: "nats",
		Binary:    in.Sidecar.BinaryPath,
		Args:      []string{"sc", "ktsp", "nats"},
		ConfigDir: filepath.Join("sidecars", in.Sidecar.Name),
	})
	if err := catalog.SaveManifest(entry.HomePath, manifest); err != nil {
		return api.UniverseInfo{}, err
	}
	notifyUniversesChanged(ctx, "sidecars-changed", in.Universe)
	return manifest.ToUniverseInfo(in.Universe, entry.HomePath, false, nil), nil
}

// RemoveSidecar removes a sidecar from an existing universe's Univers.yaml.
func (s *Service) RemoveSidecar(ctx context.Context, in api.RemoveSidecarInput) (api.UniverseInfo, error) {
	if in.Universe == "" {
		return api.UniverseInfo{}, fmt.Errorf("universe is required")
	}
	if in.Sidecar == "" {
		return api.UniverseInfo{}, fmt.Errorf("sidecar is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	entry, err := s.registry.Find(in.Universe)
	if err != nil {
		return api.UniverseInfo{}, fmt.Errorf("universe %q not in registry: %w", in.Universe, err)
	}
	manifest, err := catalog.LoadManifest(entry.HomePath)
	if err != nil {
		return api.UniverseInfo{}, fmt.Errorf("load Univers.yaml for %q: %w", in.Universe, err)
	}
	idx := -1
	for i, sc := range manifest.Sidecars {
		if sc.Name == in.Sidecar {
			idx = i
			break
		}
	}
	if idx < 0 {
		return api.UniverseInfo{}, fmt.Errorf("sidecar %q is not a member of universe %q", in.Sidecar, in.Universe)
	}
	manifest.Sidecars = append(manifest.Sidecars[:idx], manifest.Sidecars[idx+1:]...)
	if err := catalog.SaveManifest(entry.HomePath, manifest); err != nil {
		return api.UniverseInfo{}, err
	}
	notifyUniversesChanged(ctx, "sidecars-changed", in.Universe)
	return manifest.ToUniverseInfo(in.Universe, entry.HomePath, false, nil), nil
}

func (s *Service) DeleteUniverse(ctx context.Context, in api.DeleteUniverseInput) (api.DeleteUniverseOutput, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, err := s.registry.Find(in.Name)
	if err != nil {
		return api.DeleteUniverseOutput{Archived: false}, nil // idempotent
	}
	if entry.Protected {
		return api.DeleteUniverseOutput{}, fmt.Errorf("universe %q is protected; cannot be deleted", in.Name)
	}
	// A process inside a scenario case archives only the case's own universes:
	// a registry that names a universe outside the case is not the case's (the
	// person's installation, reached by a resolution that left the sandbox) -
	//
	if box, ok := cenv.ScenarioSandbox(); ok && !cenv.Within(entry.HomePath, box) {
		return api.DeleteUniverseOutput{}, fmt.Errorf(
			"universe %q lives at %s, outside the scenario case %s this process runs in: a case archives only its own universes (registry %s)",
			in.Name, entry.HomePath, box, s.registry.Path())
	}

	// A universe whose hub is still running holds its own folder open. On
	// Windows that turns the archive rename into a bare "Access denied"; on
	// Linux the rename SUCCEEDS and the live hub keeps writing into a folder
	// the user believes is gone. Refuse up front, and name the remedy.
	//
	// The lockfile is the cross-process truth: this service runs in the admin
	// hub, which has no visibility into whichever GUI or CLI activated the
	// universe. Note the GUI activates IMPLICITLY on any listing call, so
	// "I only looked at it" is enough to leave a hub running.
	if pid, running := runningHubPID(entry.HomePath); running {
		return api.DeleteUniverseOutput{}, fmt.Errorf(
			"universe %q is still running (ktw-hub pid %d); deactivate it first", in.Name, pid)
	}

	// Move folder to <parent>/.archived/<name>/
	parent := filepath.Dir(entry.HomePath)
	archiveDir := filepath.Join(parent, ".archived")
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		return api.DeleteUniverseOutput{}, fmt.Errorf("create archive dir: %w", err)
	}
	archivePath := filepath.Join(archiveDir, entry.Name)
	if err := os.Rename(entry.HomePath, archivePath); err != nil {
		// The running-hub case is handled above, so a failure here means some
		// OTHER process holds the folder (an editor, a terminal sitting in it,
		// an antivirus scan). The raw OS error alone is not actionable.
		return api.DeleteUniverseOutput{}, fmt.Errorf(
			"archive universe folder: %w (a process is holding %s open — close anything using it and retry)",
			err, entry.HomePath)
	}
	if _, err := s.registry.Remove(in.Name); err != nil {
		return api.DeleteUniverseOutput{}, fmt.Errorf("remove from registry: %w", err)
	}
	notifyUniversesChanged(ctx, "deleted", in.Name)
	return api.DeleteUniverseOutput{Archived: true, ArchivePath: archivePath}, nil
}

// runningHubPID reports the PID of a LIVE ktw-hub serving the universe rooted
// at homePath, via the lockfile the hub maintains at <DataDir>/hub/hub.lock.
//
// A lockfile whose PID is dead is STALE (the hub crashed without cleaning up)
// and deliberately does not count — otherwise one crash would make a universe
// permanently undeletable.
func runningHubPID(homePath string) (int, bool) {
	lf, err := hubclient.ReadLockFile(application.EnvOf(homePath).DataDir())
	if err != nil || lf == nil {
		return 0, false
	}
	if !hubclient.ProcessAlive(lf.PID) {
		return 0, false
	}
	return lf.PID, true
}

func (s *Service) ScanInventory(ctx context.Context, in api.ScanInventoryInput) (api.ScanInventoryOutput, error) {
	out := api.ScanInventoryOutput{}

	// Scan logic lives in catalog/scan.go and is shared with ktw-gui's
	// in-process orchestrator so both code paths use identical precedence,
	// dedupe, and exclude rules.
	//
	// probeVersion spawns each binary (`<bin> version`, 3s cap each). Some
	// binaries do full app init on `version` and take 3-8s, so probing them
	// SEQUENTIALLY summed past the KTSP request timeout (sidecar-list hung).
	// Probe concurrently instead — total ≈ the slowest single probe, not the
	// sum. Each goroutine writes its own pre-sized slot, so order is preserved
	// and there is no shared-state race.
	scanned := catalog.ScanLocal(catalog.DefaultScanRoots())
	var wg sync.WaitGroup
	for i := range scanned {
		wg.Add(1)
		go func(e *catalog.SidecarsYamlEntry) {
			defer wg.Done()
			e.Version = probeVersion(e.BinaryPath)
		}(&scanned[i])
	}
	wg.Wait()

	// Write-through to sidecars.yaml (cross-universe global catalog). Failure
	// is non-fatal: the freshly-scanned result is still returned to the caller;
	// only the persistence is best-effort. Only SCANNED rows go through here;
	// MergeInventory keeps every managed row (an install) as it is.
	var recorded []catalog.SidecarsYamlEntry
	if path, err := catalog.DefaultSidecarsPath(); err == nil {
		if merged, _, err := catalog.PersistInventory(path, scanned); err == nil {
			recorded = merged.Sidecars
		} else if sy, err := catalog.LoadSidecarsYaml(path); err == nil {
			recorded = sy.Sidecars
		}
	}

	// What a universe can be composed from: the INSTALLED items first (the
	// versioned store, which no scan can see), then discovered dev binaries,
	// never the control plane. On a machine built from the channel the scan
	// alone is exactly the control-plane folder — the editor offered ktw-hub +
	// ktw-univers and none of the sidecars the user had just installed.
	for _, e := range catalog.AvailableSidecars(recorded, scanned, controlplane.IsInfra) {
		out.Sidecars = append(out.Sidecars, api.SidecarBinary{
			Name:       e.Name,
			BinaryPath: e.BinaryPath,
			Version:    e.Version,
			Source:     e.Source,
		})
	}

	return out, nil
}

// probeVersion runs `<binary> version` and returns the trimmed first line, or
// empty string if unavailable. Best-effort; failures are silent.
//
// The probe is bounded by a hard timeout: a misbehaving binary that ignores
// the `version` verb and blocks (e.g. boots a server instead of printing and
// exiting) must NOT be able to wedge scan-inventory, which probes every ktw-*
// it discovers. On timeout the child is killed and an empty version returned.
func probeVersion(binary string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "version")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	first := strings.SplitN(string(out), "\n", 2)[0]
	return strings.TrimSpace(first)
}

// Compile-time interface satisfaction check.
var _ api.OrchestratorService = (*Service)(nil)

// validUniverseName checks ^[a-z0-9-]{1,64}$ for slug-style names.
func validUniverseName(name string) bool {
	if len(name) == 0 || len(name) > 64 {
		return false
	}
	for _, c := range name {
		switch {
		case c >= 'a' && c <= 'z':
		case c >= '0' && c <= '9':
		case c == '-':
		default:
			return false
		}
	}
	return true
}

// tagsOf is the universe's tags as given, trimmed, each once, in order.
func tagsOf(in []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, t := range in {
		if t = strings.TrimSpace(t); t != "" && !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	return out
}
