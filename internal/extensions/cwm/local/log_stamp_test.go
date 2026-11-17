// SPDX-License-Identifier: Apache-2.0

package local

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

// Booting CWM twice in one process stamps a log line with the universe and
// the project id once, not once per boot.
func TestStampDefaultLoggerStampsOncePerLine(t *testing.T) {
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })
	var buf bytes.Buffer
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))

	stampDefaultLogger("u-example", "p-1")
	stampDefaultLogger("u-example", "p-1")
	stampDefaultLogger("u-example", "p-2")
	slog.Info("hello")

	line := buf.String()
	if n := strings.Count(line, "universe="); n != 1 {
		t.Fatalf("universe= appears %d times in %q, want 1", n, line)
	}
	if n := strings.Count(line, "project_id="); n != 1 || !strings.Contains(line, "project_id=p-2") {
		t.Fatalf("want project_id=p-2 once, got %q", line)
	}
}
