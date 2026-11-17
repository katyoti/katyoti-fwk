// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/atomicio"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	"github.com/katyoti/katyoti-fwk/internal/core/oslock"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// THE INCIDENTS' LIST - what the reconciler cannot
// repair safely, kept as a system list, never a to-do: one record per check
// and element (deduplicated by its key), in the project's own `.katyoti/`
// (`planification/incidents.json` - uncommitted, recomputable: a sweep raises
// every open one again), never under modules/. Each sweep folds its incidents
// in (foldIncidents): one seen again is updated, one seen no more counts a
// clean sweep, the second clean sweep in a row closes it. The repairs'
// attempts and the day's puts are kept beside, so the flap limit and the
// day's limit hold across a restart.

const (
	incidentsFolder   = "planification"
	incidentsFile     = "incidents.json"
	incidentsLockFile = ".incidents.lock"
	// closedKeep is how long a closed incident stays in the list.
	closedKeep = 7 * 24 * time.Hour
)

// incidentsDoc is the incidents' file.
type incidentsDoc struct {
	Incidents []api.IncidentRecord `json:"incidents"`
	// Attempts are the repairs applied to each element (repair:element).
	Attempts map[string]int `json:"attempts,omitempty"`
	// Day, Puts and Stopped are the day's count of to-dos the repairs put and
	// why the reconciler switched itself to dry-run: a
	// restart the same day reads them back, so the day's limit holds.
	Day     string `json:"day,omitempty"`
	Puts    int    `json:"puts,omitempty"`
	Stopped string `json:"stopped,omitempty"`
	// Repaired are the repairs applied, kept closedKeep (: an
	// increment's Blockers tab lists those on its work).
	Repaired []api.RepairDone `json:"repaired,omitempty"`
}

// foldRepairs adds a sweep's applied repairs to the kept ones at now, and
// drops those older than closedKeep. Pure.
func foldRepairs(kept []api.RepairDone, sweep []api.Repair, now time.Time) []api.RepairDone {
	out := make([]api.RepairDone, 0, len(kept)+len(sweep))
	for _, r := range kept {
		if at, err := time.Parse(time.RFC3339, r.At); err == nil && now.Sub(at) > closedKeep {
			continue
		}
		out = append(out, r)
	}
	at := now.UTC().Format(time.RFC3339)
	for _, r := range sweep {
		if r.Applied {
			out = append(out, api.RepairDone{Repair: r, At: at})
		}
	}
	return out
}

// Repaired is the repairs the reconciler applied in the week its list keeps
// them; none when there is no list yet.
func Repaired(ctx context.Context) ([]api.RepairDone, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return nil, err
	}
	path := incidentsPath(ctx, cwm)
	if path == "" {
		return []api.RepairDone{}, nil
	}
	doc, err := readIncidents(path)
	if err != nil {
		return nil, err
	}
	return foldRepairs(doc.Repaired, nil, time.Now()), nil
}

// foldIncidents folds one sweep's incidents (deduplicated) into the list at
// now: a record per key, open ones seen again updated, the others counting a
// clean sweep and closed at the second; a closed one raised again is opened
// anew. Closed records older than closedKeep are dropped. Pure.
func foldIncidents(list []api.IncidentRecord, seen []api.Incident, now time.Time) []api.IncidentRecord {
	at := now.UTC().Format(time.RFC3339)
	byKey := map[string]int{}
	out := make([]api.IncidentRecord, 0, len(list)+len(seen))
	for _, r := range list {
		if r.Closed != "" {
			if c, err := time.Parse(time.RFC3339, r.Closed); err == nil && now.Sub(c) > closedKeep {
				continue
			}
		}
		if _, dup := byKey[r.Key]; dup {
			continue
		}
		byKey[r.Key] = len(out)
		out = append(out, r)
	}
	now1 := map[string]bool{}
	for _, inc := range seen {
		if now1[inc.Key] {
			continue
		}
		now1[inc.Key] = true
		i, ok := byKey[inc.Key]
		if !ok {
			byKey[inc.Key] = len(out)
			out = append(out, api.IncidentRecord{Incident: inc, Since: at, Seen: at, Sweeps: 1})
			continue
		}
		r := &out[i]
		if r.Closed != "" {
			*r = api.IncidentRecord{Incident: inc, Since: at, Seen: at, Sweeps: 1}
			continue
		}
		r.Incident, r.Seen, r.Clean = inc, at, 0
		r.Sweeps++
	}
	for i := range out {
		r := &out[i]
		if r.Closed != "" || now1[r.Key] {
			continue
		}
		r.Clean++
		if r.Clean >= api.IncidentCleanSweeps {
			r.Closed = at
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// openIncidents are the open records of a list.
func openIncidents(list []api.IncidentRecord) []api.IncidentRecord {
	out := []api.IncidentRecord{}
	for _, r := range list {
		if r.Closed == "" {
			out = append(out, r)
		}
	}
	return out
}

// incidentsPath is the incidents' file of the project CWM serves: in its own
// `.katyoti/`, never under its modules folder; "" when no module names the
// project.
func incidentsPath(ctx context.Context, cwm cwmapi.CWMService) string {
	list, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Capability: cwmapi.CapModule})
	if err != nil || len(list.Objects) == 0 {
		return ""
	}
	r, err := cwm.ObjectResolve(ctx, cwmapi.ObjectResolveInput{Ref: list.Objects[0].ID})
	if err != nil || r.ProjectRoot == "" {
		return ""
	}
	return filepath.Join(application.EnvOf(r.ProjectRoot).GetHomeSubDir(incidentsFolder), incidentsFile)
}

// Incidents is the open incidents of the list the reconciler keeps; none
// when there is no list yet.
func Incidents(ctx context.Context) ([]api.IncidentRecord, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return nil, err
	}
	path := incidentsPath(ctx, cwm)
	if path == "" {
		return []api.IncidentRecord{}, nil
	}
	doc, err := readIncidents(path)
	if err != nil {
		return nil, err
	}
	return openIncidents(doc.Incidents), nil
}

func readIncidents(path string) (incidentsDoc, error) {
	raw, err := atomicio.ReadAtomic(path)
	if errors.Is(err, fs.ErrNotExist) {
		return incidentsDoc{}, nil
	}
	if err != nil {
		return incidentsDoc{}, err
	}
	var doc incidentsDoc
	if err := config.DecodeBytes(raw, marshal.JSON, &doc); err != nil {
		return incidentsDoc{}, fmt.Errorf("%s: %w", path, err)
	}
	return doc, nil
}

// editIncidents rewrites the incidents' file under its lock.
func editIncidents(path string, edit func(*incidentsDoc)) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	lock := filepath.Join(dir, incidentsLockFile)
	deadline := time.Now().Add(holdsLockWait)
	for {
		l, err := oslock.TryLock(lock)
		if err == nil {
			_ = l.Note(fmt.Sprintf("pid %d", os.Getpid()))
			defer l.Unlock()
			break
		}
		if !errors.Is(err, oslock.ErrLocked) {
			return err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("the incidents' list is locked by %s (%s)", oslock.Note(lock), lock)
		}
		time.Sleep(50 * time.Millisecond)
	}
	doc, err := readIncidents(path)
	if err != nil {
		return err
	}
	edit(&doc)
	raw, err := marshal.Marshal(doc, marshal.JSON)
	if err != nil {
		return err
	}
	return atomicio.WriteAtomic(path, raw, 0o644)
}
