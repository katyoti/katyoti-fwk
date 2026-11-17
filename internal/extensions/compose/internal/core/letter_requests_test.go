// SPDX-License-Identifier: Apache-2.0

package core

import (
	"bytes"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	planapi "github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// THE LETTER'S REQUESTS FIGURE IS CHECKED: the build counts
// the distinct requests the changes of every change log a written « Quoi de
// neuf » covers name - each change log a release's page named by its version
// - and hands it to the letter's check (authoring.CheckLetterFigures refuses a
// drifting figure); when the changes cannot be listed the figure is left
// unchecked, and said.
func TestTheLetterRequestsFigureIsCountedFromTheCoveredChanges(t *testing.T) {
	dir := t.TempDir()
	covers := []string{filepath.Join(dir, "changelog", "2026.10.md"), filepath.Join(dir, "changelog", "1.006.md")}
	byRelease := map[string][]planapi.Change{
		"2026.10": {{ChangeFields: planapi.ChangeFields{Requests: []string{"m.F-039", "m.F-038"}}}},
		"1.006":   {{ChangeFields: planapi.ChangeFields{Requests: []string{"m.RQ-273", "m.F-039"}}}, {ChangeFields: planapi.ChangeFields{Requests: []string{"m.RQ-021"}}}},
	}
	var asked []string
	list := func(release string) ([]planapi.Change, error) {
		asked = append(asked, release)
		return byRelease[release], nil
	}
	var out bytes.Buffer
	if got := letterRequests(nil, covers, list, &out); got != 4 {
		t.Fatalf("the requests the covered changes name: %d; want 4, each once", got)
	}
	if strings.Join(asked, ",") != "2026.10,1.006" {
		t.Errorf("the releases asked: %v; want each change log's version", asked)
	}

	// No changes service, or a failing one: unchecked, and said.
	out.Reset()
	if got := letterRequests(nil, covers, nil, &out); got != 0 || !strings.Contains(out.String(), "not checked") {
		t.Errorf("without a changes service: %d %q; want 0, said", got, out.String())
	}
	out.Reset()
	failing := func(string) ([]planapi.Change, error) { return nil, errors.New("hub down") }
	if got := letterRequests(nil, covers, failing, &out); got != 0 || !strings.Contains(out.String(), "hub down") {
		t.Errorf("a failing changes service: %d %q; want 0, said", got, out.String())
	}
}
