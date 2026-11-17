// SPDX-License-Identifier: Apache-2.0

// Package ucobj reads a module's use cases from their objects and records
// verify's runs as objects.
//
// THE SPLIT IT HOLDS. A use case is three different kinds of thing, and they
// do not live in the same place:
//
//   - the use-case itself       -> one `use-case` object
//   - each acceptance criterion -> one `acceptance-criterion` object
//   - the STATUSES              -> a `test-run` object, one per run
//
// The third is the point: only a run produces a verdict, so a status never
// sits on the use case or the criterion — it goes onto the run that produced
// it, and the latest run per use case is what is read back.
//
// Each AC is its OWN object because the method's atomic unit is the
// acceptance criterion: a list inside one object cannot be graded
// individually, and an object can.
package ucobj

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/marshal"

	gcapi "github.com/katyoti/katyoti-fwk/internal/extensions/code/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/code/internal/ucfile"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// TypeUseCase and TypeAcceptanceCriterion are the object types produced.
const (
	TypeUseCase             = "use-case"
	TypeAcceptanceCriterion = "acceptance-criterion"
)

// Verdict is a per-use-case run result together with WHEN it was produced.
//
// The timestamp travels with the verdict, not with the use-case. "When was
// this last verified?" is a fact about the RUN; a use-case object carrying it
// would be carrying verdict provenance, which is the thing this package exists
// to move off the criterion. The manifest's `last_verified_at` is rendered
// from here.
type Verdict struct {
	gcapi.TestRunVerdict
	// RanAt is when the run that produced this verdict happened. Zero when
	// nothing has ever verified this use-case — which is a real state, not a
	// missing value, and renders as absent rather than as the epoch.
	RanAt time.Time
}

// ObjectID is the module-qualified id for an authored local id.
func ObjectID(module, local string) string { return module + "." + local }

// FromObjects is the inverse: it rebuilds UC files from objects so the
// existing renderers can be driven from the object graph and their output
// compared byte for byte against the file-sourced original.
//
// verdicts may be nil — a module whose statuses were never recorded rebuilds
// with empty statuses, which is the honest result rather than an invented one.
func FromObjects(objs []cwmapi.Object, module string, verdicts map[string]Verdict) ([]*ucfile.UCFile, error) {
	byUC := map[string]*ucfile.UCFile{}
	criteria := map[string][]cwmapi.Object{}

	for _, o := range objs {
		switch o.Type {
		case TypeUseCase:
			f, err := useCaseFrom(o, module)
			if err != nil {
				return nil, err
			}
			byUC[o.ID] = f
		case TypeAcceptanceCriterion:
			criteria[o.ParentID()] = append(criteria[o.ParentID()], o)
		}
	}

	out := make([]*ucfile.UCFile, 0, len(byUC))
	for id, f := range byUC {
		acs := criteria[id]
		// NUMERIC order, not lexicographic. `UC-058-AC10` sorts BEFORE
		// `UC-058-AC2` as a string, which silently reorders the criteria of
		// every use-case that has ten or more. Caught by the round-trip over
		// the real corpus; a fixture with three criteria would have passed.
		sort.Slice(acs, func(i, j int) bool { return acIndexLess(acs[i].ID, acs[j].ID) })
		v := verdicts[f.Meta.ID]
		for _, a := range acs {
			ac, err := criterionFrom(a)
			if err != nil {
				return nil, err
			}
			if s, ok := v.ACs[ac.ID]; ok {
				ac.Status = string(s)
			}
			f.Meta.AcceptanceCriteria = append(f.Meta.AcceptanceCriteria, ac)
		}
		f.Meta.StatusCache = string(v.Verdict)
		// Restored onto the RENDERING INPUT, never onto a stored object: the
		// rebuilt UCFile is what the projection consumes, and the projection
		// reports when the verdict was produced.
		if !v.RanAt.IsZero() {
			f.Meta.LastVerifiedAt = v.RanAt.UTC().Format(time.RFC3339)
		}
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Meta.ID < out[j].Meta.ID })
	return out, nil
}

func useCaseFrom(o cwmapi.Object, module string) (*ucfile.UCFile, error) {
	var b ucBody
	if err := json.Unmarshal([]byte(bodyString(o)), &b); err != nil {
		return nil, fmt.Errorf("ucobj: unmarshal use-case %s: %w", o.ID, err)
	}
	id := strings.TrimPrefix(o.ID, module+".") // the authored id IS the UC-NNN
	severity, _ := o.Capabilities["checkable"]["severity"].(string)
	return &ucfile.UCFile{
		Meta: ucfile.UCMeta{
			ID:             id,
			Kind:           ucfile.KindUseCase,
			Title:          o.Name,
			Description:    o.Description,
			Module:         module,
			Severity:       severity,
			FailDeferredBy: gcapi.DeferredByLocal(o.EdgeTo(gcapi.EdgeDeferredBy)),
		},
		Body: o.Body,
	}, nil
}

func criterionFrom(o cwmapi.Object) (ucfile.AC, error) {
	var b acBody
	if err := json.Unmarshal([]byte(bodyString(o)), &b); err != nil {
		return ucfile.AC{}, fmt.Errorf("ucobj: unmarshal criterion %s: %w", o.ID, err)
	}
	note, _ := o.Capabilities["checkable"]["note"].(string)
	return ucfile.AC{
		ID:      o.ID[strings.Index(o.ID, ".")+1:], // the authored id IS the AC id
		Claim:   o.Name,
		Kind:    b.Kind,
		Layer:   b.Layer,
		TestRef: b.TestRef,
		Note:    note,
	}, nil
}

// acIndexLess orders two acceptance-criterion object ids the way a reader
// does: AC1 < AC2 < AC10, and a split child AC2.1 directly after AC2.
//
// Splits EXTEND rather than renumber (the ID-stability rule), so the segments
// after `AC` are a dotted number and have to be compared component by
// component. Ids that do not parse fall back to string order, which keeps the
// sort total rather than silently grouping them.
func acIndexLess(a, b string) bool {
	ai, aok := acIndex(a)
	bi, bok := acIndex(b)
	if !aok || !bok {
		return a < b
	}
	for k := 0; k < len(ai) && k < len(bi); k++ {
		if ai[k] != bi[k] {
			return ai[k] < bi[k]
		}
	}
	return len(ai) < len(bi)
}

// acIndex extracts the dotted numeric tail of an AC id: `-AC10.2`
// -> [10, 2]. Returns ok=false when there is no `-AC` segment to read.
func acIndex(id string) ([]int, bool) {
	i := strings.LastIndex(id, "-AC")
	if i < 0 {
		return nil, false
	}
	var out []int
	for _, part := range strings.Split(id[i+len("-AC"):], ".") {
		n, err := strconv.Atoi(part)
		if err != nil {
			return nil, false
		}
		out = append(out, n)
	}
	return out, len(out) > 0
}

// bodyString reads the type's fields through the SAME typed accessor the
// projection's loader uses, so this package cannot accidentally read an
// encoding nothing else can.
func bodyString(o cwmapi.Object) string {
	b := o.FieldsJSON()
	if len(b) == 0 {
		return "{}"
	}
	return string(b)
}

// ucBody is the use-case type's fields. Its id, title, description and DoD
// prose are the object core, its severity the checkable capability's, the
// decision deferring it a `deferred-by` edge.
type ucBody struct{}

// acBody is the acceptance-criterion type's fields. Its id and claim are the
// object core (id, name), its note the checkable capability's.
//
// There is NO status field, and that is deliberate: see the package comment.
type acBody struct {
	Kind    string `json:"kind,omitempty"`
	Layer   string `json:"layer"`
	TestRef string `json:"test_ref"`
}

// cwmObject is a local alias so tests and helpers can name the object type
// without repeating the import path.
type cwmObject = cwmapi.Object

// LoadDir reads every object file under dir and rebuilds the use-cases.
//
// It reads the FILES rather than going through CWMService on purpose: this is
// the projection's read path, and the projection runs as a CLI verb against a
// checked-out repo, where the objects are just files. The service's index is a
// rebuildable view over exactly these files, so reading them directly cannot
// disagree with it.
func LoadDir(dir, module string, verdicts map[string]Verdict) ([]*ucfile.UCFile, error) {
	var objs []cwmapi.Object
	paths := map[string]string{} // use-case id → its object file, for findings
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			// `_`-prefixed folders are never scanned — one convention, applied
			// to files and folders alike.
			if strings.HasPrefix(d.Name(), "_") && path != dir {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), objectFileSuffix) {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		var o cwmapi.Object
		if err := marshal.Unmarshal(raw, marshal.YAML, &o); err != nil {
			return fmt.Errorf("decode %s: %w", path, err)
		}
		objs = append(objs, o)
		paths[strings.TrimPrefix(o.ID, module+".")] = path
		return nil
	})
	if err != nil {
		return nil, err
	}
	// Deterministic input -> deterministic output. WalkDir is already sorted,
	// but saying so here means a future change of reader cannot silently make
	// the projection order-dependent.
	sort.Slice(objs, func(i, j int) bool { return objs[i].ID < objs[j].ID })
	files, err := FromObjects(objs, module, verdicts)
	if err != nil {
		return nil, err
	}
	for _, f := range files {
		f.Path = paths[f.Meta.ID]
	}
	return files, nil
}

// Run is one verify invocation's result, as a `test-run` object records it.
type Run struct {
	RanAt       time.Time
	LayerFilter string
	UCFilter    string
	Verdicts    map[string]gcapi.TestRunVerdict
	UCTotal     int
}

// WriteRun records a run as a NEW `test-run` object through CWM's Object API
// and returns the file CWM wrote it to. Runs accumulate — a verdict belongs to
// the run that produced it, so nothing is overwritten and no use-case or
// criterion is touched; LoadVerdicts reads the latest by `ran_at`.
//
// THROUGH CWM, NEVER BESIDE IT. The run is an object like any other: CWM
// validates it against its type, holds it to the type's size budget, indexes
// it and publishes its creation. A run is therefore written where CWM's
// workspace is, and nowhere a caller's path happens to point — there is no
// file-writing fallback, so a caller without the service records nothing.
func WriteRun(ctx context.Context, cwm cwmapi.CWMService, module string, r Run) (string, error) {
	if cwm == nil {
		return "", fmt.Errorf("record the test-run: no CWM service — a run is recorded through CWM's Object API")
	}
	totals := gcapi.TestRunTotals{UCTotal: r.UCTotal}
	for _, v := range r.Verdicts {
		switch string(v.Verdict) {
		case ucfile.StatusPass:
			totals.Pass++
		case ucfile.StatusFail:
			totals.Fail++
		case ucfile.StatusFailDeferred:
			totals.FailDeferred++
		case ucfile.StatusNoTest:
			totals.NoTest++
		case ucfile.StatusSkipped:
			totals.Skipped++
		}
	}
	fields := map[string]any{
		"module":       module,
		"ran_at":       r.RanAt.UTC().Format(time.RFC3339),
		"layer_filter": r.LayerFilter,
		"totals":       totals,
		"verdicts":     r.Verdicts,
	}
	if r.UCFilter != "" {
		fields["uc_filter"] = r.UCFilter
	}
	body, err := json.Marshal(fields)
	if err != nil {
		return "", fmt.Errorf("marshal test-run: %w", err)
	}
	local := "test-run-" + r.RanAt.UTC().Format("20060102-150405")
	o := cwmapi.NewRecordObject(module, local, "test-run", module,
		fmt.Sprintf("module verify %s — %s", r.LayerFilter, r.RanAt.UTC().Format("2006-01-02 15:04")), body)
	out, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: module, Object: o})
	if err != nil {
		if errors.Is(err, cwmapi.ErrAlreadyExists) {
			return "", fmt.Errorf("test-run %s already exists — two runs in one second: %w", o.ID, err)
		}
		return "", fmt.Errorf("create test-run %s: %w", o.ID, err)
	}
	return out.Path, nil
}

// objectFileSuffix marks a file as a CWM object. It is a FORMAT marker and
// carries no identity — see cwm's ObjectFileSuffix, which this mirrors.
const objectFileSuffix = ".cwm.yaml"

// LoadVerdicts reads the module's test-run objects from its files and returns
// the latest verdict per use-case (see LatestVerdicts).
func LoadVerdicts(moduleDir, module string) (map[string]Verdict, error) {
	var runs []cwmapi.Object
	err := filepath.WalkDir(moduleDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), "_") && path != moduleDir {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), objectFileSuffix) {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		var o cwmapi.Object
		if err := marshal.Unmarshal(raw, marshal.YAML, &o); err != nil {
			return fmt.Errorf("decode %s: %w", path, err)
		}
		runs = append(runs, o)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return LatestVerdicts(runs)
}

// LatestVerdicts returns the latest verdict per use-case among the test-run
// objects in objs (objects of any other type are ignored). It is THE reading
// of a use case's status: the charter, the manifest and the test plan all
// take it from here, whether their objects came from the files or from the
// CWM service.
//
// LATEST WINS, by the run's own `ran_at` rather than by file order: runs
// accumulate, and which one is current is a fact in the content, never in the
// filename. No run returns an empty map — no verdicts is a real state, and it
// renders as "no-test" rather than as an error.
func LatestVerdicts(objs []cwmapi.Object) (map[string]Verdict, error) {
	out := map[string]Verdict{}
	latest := map[string]time.Time{}
	for _, o := range objs {
		if o.Type != "test-run" {
			continue
		}
		bodyRaw := o.FieldsJSON()
		if len(bodyRaw) == 0 {
			continue
		}
		var body struct {
			RanAt    string                          `json:"ran_at"`
			Verdicts map[string]gcapi.TestRunVerdict `json:"verdicts"`
		}
		if err := json.Unmarshal(bodyRaw, &body); err != nil {
			return nil, fmt.Errorf("decode test-run %s: %w", o.ID, err)
		}
		ranAt, _ := time.Parse(time.RFC3339, body.RanAt)
		for ucID, v := range body.Verdicts {
			if prev, seen := latest[ucID]; seen && !ranAt.After(prev) {
				continue
			}
			latest[ucID] = ranAt
			out[ucID] = Verdict{TestRunVerdict: v, RanAt: ranAt}
		}
	}
	return out, nil
}
