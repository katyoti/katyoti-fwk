// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/projectfile"
	"github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// : every change of an object's shape ships its migration step in the
// same commit. The shape of the data is what the corpus is judged
// against - each capability's and type's schema, each workflow's states, each
// type's fields and capabilities - without its wording. Two histories record
// the shape's hash at each data version, append only:
// testdata/data-shape-public.golden the public shape (the packs the open-source
// tree ships), checked by every build, and testdata/data-shape.golden the
// combined shape (every pack), checked by the private build only. A shape that
// changes while the data version stays is refused, and a recorded version is
// never rewritten, so the one way through is a new step in migrate.go.
// testdata/data-shape-lines.golden holds the public shape's lines, to say what
// changed; it is published, so no private pack's line is in it.
//
//	KTW_SHAPE_UPDATE=1 go test ./internal/extensions/cwm/internal/core/ -run TestDataShapeHasItsMigration
//
// records the current version's shape in both histories once its step is
// added.

const (
	shapeHistoryFile       = "testdata/data-shape.golden"
	publicShapeHistoryFile = "testdata/data-shape-public.golden"
	shapeLinesFile         = "testdata/data-shape-lines.golden"
)

// shapeLines is the data's shape, one sorted line per schema, workflow and
// type, wording left out.
func shapeLines(cfg *CWMServiceConfig) []string {
	var lines []string
	for uri, raw := range cfg.CatalogTypes.CapabilitySchemas {
		var v any
		if err := json.Unmarshal(raw, &v); err != nil {
			lines = append(lines, "schema "+uri+" unreadable")
			continue
		}
		b, _ := json.Marshal(withoutWording(v, false))
		lines = append(lines, "schema "+uri+" "+hashOf(b))
	}
	for _, w := range cfg.CatalogTypes.Workflows {
		var states []string
		for _, s := range w.States {
			states = append(states, s.Name)
		}
		sort.Strings(states)
		lines = append(lines, "workflow "+w.Name+" "+strings.Join(states, ","))
	}
	for _, ot := range cfg.CatalogTypes.Objects {
		caps := slices.Clone(ot.Capabilities)
		sort.Strings(caps)
		lines = append(lines, "type "+ot.Name+" fields="+ot.Fields+" caps="+strings.Join(caps, ","))
	}
	sort.Strings(lines)
	return lines
}

// withoutWording drops the keywords that word a schema - never a property that
// happens to bear their name (inside `properties`, keys are names).
func withoutWording(v any, names bool) any {
	switch x := v.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, y := range x {
			if !names {
				switch k {
				case "description", "title", "$comment", "examples":
					continue
				}
			}
			out[k] = withoutWording(y, !names && (k == "properties" || k == "$defs" || k == "patternProperties"))
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, y := range x {
			out[i] = withoutWording(y, false)
		}
		return out
	}
	return v
}

func hashOf(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:8])
}

// shapeHistory reads the recorded shape hash of each data version.
func shapeHistory(text string) (map[int]string, error) {
	out := map[int]string{}
	for _, l := range strings.Split(text, "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		f := strings.Fields(l)
		v, err := strconv.Atoi(f[0])
		if len(f) != 2 || err != nil {
			return nil, fmt.Errorf("%q is not '<data version> <hash>'", l)
		}
		out[v] = f[1]
	}
	return out, nil
}

// shapeVerdict says whether the shape at version is the one recorded for it:
// "" when it is, "record" when the version has none yet, else why it fails.
func shapeVerdict(history map[int]string, version int, hash string) string {
	got, ok := history[version]
	switch {
	case !ok:
		return "record"
	case got == hash:
		return ""
	}
	return fmt.Sprintf("the shape of the data changed but the data version is still %d (recorded %s, now %s): "+
		"a change of an object's shape ships its migration step in the same commit - add it to migrationSteps (migrate.go), "+
		"then record the new version's shape: KTW_SHAPE_UPDATE=1 go test ./internal/extensions/cwm/internal/core/ -run TestDataShapeHasItsMigration", version, got, hash)
}

// shapeCheck judges lines against the history in file at version: "" when the
// recorded shape holds, else why not. With update, a version not yet recorded
// is appended - a recorded one is never rewritten.
func shapeCheck(t *testing.T, file string, version int, lines []string, update bool) string {
	t.Helper()
	if len(lines) < 100 {
		t.Fatalf("only %d shape lines - the capture is broken, not the packs", len(lines))
	}
	hash := hashOf([]byte(strings.Join(lines, "\n")))
	raw, err := os.ReadFile(file)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	history, err := shapeHistory(string(raw))
	if err != nil {
		t.Fatalf("%s: %v", file, err)
	}
	verdict := shapeVerdict(history, version, hash)
	switch {
	case verdict == "record" && update:
		head := string(raw)
		if head == "" {
			head = "# The shape of the project's data at each data version (data_shape_test.go): append only.\n"
		}
		if !strings.HasSuffix(head, "\n") {
			head += "\n"
		}
		if err := os.WriteFile(file, []byte(head+fmt.Sprintf("%d %s\n", version, hash)), 0o644); err != nil {
			t.Fatal(err)
		}
		return ""
	case verdict == "record":
		return fmt.Sprintf("data version %d has no recorded shape in %s: record it with KTW_SHAPE_UPDATE=1 go test ./internal/extensions/cwm/internal/core/ -run TestDataShapeHasItsMigration", version, file)
	case verdict != "" && update:
		t.Fatalf("not recorded - a version's shape is never rewritten (%s): %s", file, verdict)
	}
	return verdict
}

func TestDataShapeHasItsMigration(t *testing.T) {
	update := os.Getenv("KTW_SHAPE_UPDATE") != ""
	version := dataVersionOf(migrationSteps)
	// The public shape: the packs without the private build's, chosen by the
	// pack each line comes from - in the open-source tree, every pack.
	public, _ := realCorpusConfigOf(t, privateShapePacks)
	lines := shapeLines(public)
	if verdict := shapeCheck(t, publicShapeHistoryFile, version, lines, update); verdict != "" {
		was, _ := os.ReadFile(shapeLinesFile)
		t.Fatalf("%s\nwhat changed:\n%s", verdict, lineDiff(strings.Split(strings.TrimSpace(string(was)), "\n"), lines))
	}
	if update {
		if err := os.WriteFile(shapeLinesFile, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if len(privateShapePacks) == 0 {
		return // the open-source build: no private part to judge
	}
	// The combined shape, every pack. The public part held, so a refusal here
	// is a change in the private part, whose lines are never published.
	all, _ := realCorpusConfig(t)
	if verdict := shapeCheck(t, shapeHistoryFile, version, shapeLines(all), update); verdict != "" {
		t.Fatalf("%s\nwhat changed: the private part of the shape (packs %v) - the public lines are unchanged", verdict, privateShapePacks)
	}
}

// privateShapePacks are the packs of the private build
// (migrate_private_test.go): the public shape and its published lines leave
// them out, the combined history covers them. The open-source build has none.
var privateShapePacks []string

// lineDiff lists the lines only one side has.
func lineDiff(was, now []string) string {
	var b strings.Builder
	for _, l := range was {
		if !slices.Contains(now, l) {
			b.WriteString("  - " + l + "\n")
		}
	}
	for _, l := range now {
		if !slices.Contains(was, l) {
			b.WriteString("  + " + l + "\n")
		}
	}
	return b.String()
}

// TestAShapeChangeWithoutAStepIsRefused: the guard on its own - a new required
// field changes the shape, a reworded description does not; the changed shape
// at the same version is refused, at the next version (a step added) recorded.
func TestAShapeChangeWithoutAStepIsRefused(t *testing.T) {
	schema := func(s string) *CWMServiceConfig {
		cfg := &CWMServiceConfig{}
		cfg.CatalogTypes.CapabilitySchemas = map[string]json.RawMessage{"ktw-agents:/data-type/cap-x": json.RawMessage(s)}
		return cfg
	}
	base := shapeLines(schema(`{"type":"object","description":"x","properties":{"title":{"type":"string","description":"a"}}}`))
	reworded := shapeLines(schema(`{"type":"object","description":"y","properties":{"title":{"type":"string","description":"b"}}}`))
	required := shapeLines(schema(`{"type":"object","description":"x","required":["title"],"properties":{"title":{"type":"string","description":"a"}}}`))
	renamed := shapeLines(schema(`{"type":"object","description":"x","properties":{"name":{"type":"string","description":"a"}}}`))
	if !slices.Equal(base, reworded) {
		t.Fatal("a reworded description changed the shape")
	}
	if slices.Equal(base, required) || slices.Equal(base, renamed) {
		t.Fatal("a new required field or a renamed property left the shape unchanged")
	}
	h := func(l []string) string { return hashOf([]byte(strings.Join(l, "\n"))) }
	history := map[int]string{1: h(base)}
	if v := shapeVerdict(history, 1, h(base)); v != "" {
		t.Fatalf("the recorded shape is refused: %s", v)
	}
	if v := shapeVerdict(history, 1, h(renamed)); v == "" || v == "record" {
		t.Fatal("a shape change with no migration step passed")
	}
	if v := shapeVerdict(history, 2, h(renamed)); v != "record" {
		t.Fatalf("the shape at the next data version (a step added) is not recorded: %q", v)
	}
}

// migratedCorpus answers the project root the corpus tests judge: the project
// itself when its data is at the code's version, else a copy of its project
// object and modules migrated by steps - the project is never written.
func migratedCorpus(t *testing.T, project string, steps []MigrationStep) string {
	t.Helper()
	f, err := projectfile.Read(project)
	if err != nil {
		t.Fatal(err)
	}
	if f.DataVersion >= dataVersionOf(steps) {
		return project
	}
	dst := t.TempDir()
	copyFile(t, filepath.Join(project, projectfile.FileName), filepath.Join(dst, projectfile.FileName))
	copyTree(t, filepath.Join(project, f.ModulesDir), filepath.Join(dst, f.ModulesDir))
	migrateCopy(t, dst, steps)
	return dst
}

// migrateCopy migrates, in place, a copy the test owns (a temp dir: never the
// project) to the version steps reach. A corpus already copied out of git
// (corpusAtTag) is migrated where it stands: a second copy of six thousand
// files costs most of a minute on Windows.
func migrateCopy(t *testing.T, dst string, steps []MigrationStep) {
	t.Helper()
	svc, err := New(stdctx.Background(), &CWMServiceConfig{WorkspaceRoot: freshWorkspace(t), ProjectRoot: dst})
	if err != nil {
		t.Fatalf("open the copy: %v", err)
	}
	defer svc.Close()
	svc.steps = steps
	out, err := svc.Migrate(stdctx.Background(), api.MigrateInput{})
	if err != nil {
		t.Fatalf("migrate the copy: %v", err)
	}
	t.Logf("corpus migrated on a copy from data version %d to %d", out.From, out.To)
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	b, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		copyFile(t, p, filepath.Join(dst, rel))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestTheCorpusIsJudgedOnAMigratedCopy: a project behind the code's data
// version is migrated on a copy - the copy's objects in the new shape, the
// project's files unchanged; one at the code's version is judged in place.
func TestTheCorpusIsJudgedOnAMigratedCopy(t *testing.T) {
	_, project := migrationFixture(t)
	if got := migratedCorpus(t, project, nil); got != project {
		t.Fatalf("a project at the code's version was copied: %s", got)
	}
	before := snapshot(t, project)
	dst := migratedCorpus(t, project, twoSteps())
	if dst == project {
		t.Fatal("a project behind the code's version was judged in place")
	}
	if after := snapshot(t, project); !mapsEqual(before, after) {
		t.Fatal("judging the corpus wrote to the project")
	}
	o, err := ReadObjectFile(filepath.Join(dst, api.DefaultModulesDir, "m", ObjectsDirName, "m.F-001"+ObjectFileSuffix))
	if err != nil {
		t.Fatal(err)
	}
	if want, _ := o.Capability("want"); want["reachability"] != "closable" || want["closable"] != true {
		t.Fatalf("the copy is not migrated: want = %v", want)
	}
	if f, _ := projectfile.Read(dst); f.DataVersion != 3 {
		t.Fatalf("the copy's data version = %d, want 3", f.DataVersion)
	}
}

// shapeProblems drops the problems that judge the code tree rather than the
// data: an edge that targets a path is checked against the project's files,
// which a migrated copy or an older release's corpus does not hold. The
// project itself, judged in place, keeps them.
func shapeProblems(problems []string) []string {
	var out []string
	for _, p := range problems {
		if !strings.Contains(p, "targets a path and") {
			out = append(out, p)
		}
	}
	return out
}
