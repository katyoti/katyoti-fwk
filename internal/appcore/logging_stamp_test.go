// SPDX-License-Identifier: Apache-2.0

package appcore

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

// A LINE READ BACK RESOLVES: universe and project id are structured
// attributes on every record, not prose someone has to grep for.
func TestStampLoggerWritesResolvableLines(t *testing.T) {
	var buf bytes.Buffer
	base := slog.New(slog.NewTextHandler(&buf, nil))
	l := StampLogger(base, "acme", "00000000-0000-7000-8000-000000000001")
	l.Info("object written", "id", "live.g1")
	line := buf.String()
	for _, want := range []string{"universe=acme", "project_id=00000000-0000-7000-8000-000000000001", "id=live.g1"} {
		if !strings.Contains(line, want) {
			t.Errorf("line lacks %q: %s", want, line)
		}
	}
	buf.Reset()
	StampLogger(base, "", "").Info("bare")
	if strings.Contains(buf.String(), "universe=") || strings.Contains(buf.String(), "project_id=") {
		t.Errorf("empty stamps must be absent, not blank: %s", buf.String())
	}
}
