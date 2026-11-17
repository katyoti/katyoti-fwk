// SPDX-License-Identifier: Apache-2.0

package backupjob

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/meta"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	inboxapi "github.com/katyoti/katyoti-fwk/internal/extensions/inbox/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/orchestrator/internal/backup"
)

// NAMING THE BACKUP TARGETS IN PLACE: the « Name the backup targets » item of
// « À faire » carries its answer form (the waiting source's Controls): the
// local folder, prefilled with the suggested one, and an optional off-PC
// folder. Sending it runs `ktw-agents:universe-backup/targets`, which writes
// UniverseBackup.yaml - the universe's file when it holds one, else the
// global layer's, the targets being the person's for every universe - and the
// item goes, the job reading the file at its next run.

// BackupServiceDef declares the backup job's acts for KTSP
// (`ktw-agents:universe-backup/<verb>`).
var BackupServiceDef = meta.ServiceDef{
	Name:    "universe-backup",
	Short:   "The universe backup job: name its target folders",
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		"Targets": {Short: "Name the backup job's local target folder and, optionally, its off-PC folder (written to UniverseBackup.yaml)"},
	},
}

// BackupTargetsInput names the job's targets.
type BackupTargetsInput struct {
	Local string `json:"local" jsonschema:"description=The local target folder off the system disk (an absolute path)"`
	OffPC string `json:"off_pc,omitempty" jsonschema:"description=The off-PC target folder (an absolute path); empty keeps none"`
}

// UniverseBackupService is the backup job's acts.
type UniverseBackupService interface {
	Targets(ctx context.Context, in BackupTargetsInput) (render.Result, error)
}

// targetsURI is the act the form runs; targetsFormID the form's element id
// (the GUI walk finds it by id).
const (
	targetsURI    = "ktw-agents:universe-backup/targets"
	targetsFormID = "universe-backup-targets"
)

// backupStore is where UniverseBackup.yaml is read and written: the global
// config layer under the universe's, the universe's file replacing the global
// one as the configuration chain does. Read from disk at each call, so a
// target named while the hub serves is seen at once.
type backupStore struct {
	global   string // <GlobalHomeDir>/config
	universe string // the universe's config folder
}

var _ UniverseBackupService = backupStore{}

// envStore is the store of the application's environment.
func envStore() backupStore {
	env := application.Env()
	if env == nil {
		return backupStore{}
	}
	return backupStore{global: filepath.Join(env.GlobalHomeDir(), "config"), universe: env.ConfigDir()}
}

// dir is the layer holding the effective file: the universe's when it has
// one, else the global one.
func (s backupStore) dir() string {
	if s.universe != "" {
		if _, err := os.Stat(filepath.Join(s.universe, config.ConfigName[backup.UniverseBackup]()+".yaml")); err == nil {
			return s.universe
		}
	}
	return s.global
}

// Load reads the effective UniverseBackup.yaml; none is the default
// configuration (on, no target).
func (s backupStore) Load(context.Context) (backup.UniverseBackup, error) {
	var cfg backup.UniverseBackup
	dir := s.dir()
	if dir == "" {
		return cfg, config.ProcessConfig(&cfg)
	}
	err := config.LoadFromDir(dir, config.ConfigName[backup.UniverseBackup](), &cfg)
	return cfg, err
}

// Targets writes the targets named into the effective file, keeping its other
// fields; the suggestion is dropped once a target is named.
func (s backupStore) Targets(ctx context.Context, in BackupTargetsInput) (render.Result, error) {
	local, off := strings.TrimSpace(in.Local), strings.TrimSpace(in.OffPC)
	if err := checkFolder("local", local, true); err != nil {
		return render.Result{OK: false, Message: err.Error()}, nil
	}
	if err := checkFolder("off_pc", off, false); err != nil {
		return render.Result{OK: false, Message: err.Error()}, nil
	}
	dir := s.dir()
	if dir == "" {
		return render.Result{OK: false, Message: "no configuration folder: UniverseBackup.yaml cannot be written"}, nil
	}
	var cfg backup.UniverseBackup
	if err := config.LoadFromDir(dir, config.ConfigName[backup.UniverseBackup](), &cfg); err != nil {
		return render.Result{OK: false, Message: err.Error()}, nil
	}
	cfg.Local, cfg.OffPC, cfg.SuggestedLocal = local, off, ""
	if err := config.SaveAtomic(dir, config.ConfigName[backup.UniverseBackup](), &cfg); err != nil {
		return render.Result{OK: false, Message: err.Error()}, nil
	}
	msg := fmt.Sprintf("Backup targets named: %s", local)
	if off != "" {
		msg += fmt.Sprintf(", off the PC %s", off)
	}
	return render.Result{OK: true, Message: msg + "."}, nil
}

// checkFolder refuses a folder that is not an absolute path; an optional one
// may be empty.
func checkFolder(field, p string, required bool) error {
	if p == "" {
		if required {
			return errors.New(field + ": name a folder")
		}
		return nil
	}
	if !filepath.IsAbs(p) {
		return fmt.Errorf("%s: %q is not an absolute path", field, p)
	}
	return nil
}

// targetsControls are the item's answer: the targets form, prefilled with the
// named or suggested local folder.
func targetsControls(config func(context.Context) (backup.UniverseBackup, error)) func(context.Context, inboxapi.WaitingItem, string) []render.Node {
	return func(ctx context.Context, _ inboxapi.WaitingItem, lang string) []render.Node {
		cfg, _ := config(ctx)
		local := cfg.Local
		if local == "" {
			local = cfg.SuggestedLocal
		}
		say := func(fr, en string) string {
			if lang == "fr" {
				return fr
			}
			return en
		}
		schema := render.Node{"name": targetsFormID, "version": 1, "root": "Targets", "types": render.Node{
			"Targets": render.Node{"type": "object", "label": say("Cibles de sauvegarde", "Backup targets"), "fields": render.Node{
				"local": render.Node{"type": "string", "label": say("Dossier local", "Local folder"),
					"description": say("Un dossier hors du disque système, copié à chaque passage", "A folder off the system disk, copied at every run")},
				"off_pc": render.Node{"type": "string", "optional": true, "label": say("Dossier hors du PC (facultatif)", "Off-PC folder (optional)"),
					"description": say("Un dossier monté hors du PC, copié une fois par jour", "A mounted folder off the PC, copied once a day")},
			}},
		}}
		submit := render.Action{Label: say("Enregistrer les cibles", "Save the targets"), URI: targetsURI,
			Args: map[string]any{"local": "${value.local}", "off_pc": "${value.off_pc}"}}
		data := render.Node{"local": local, "off_pc": cfg.OffPC}
		return []render.Node{render.AnswerForm(schema, submit, "device-floppy", data).With("id", targetsFormID)}
	}
}
