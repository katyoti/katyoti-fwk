// SPDX-License-Identifier: Apache-2.0

package appcore

import (
	"log"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
)

// SetupFileLogging routes the default slog logger + the std `log` package to
// a per-binary append log file under Env().LogDir(), bounded in size (rotated
// to <name>.log.1 past its limit, logrotate.go), so framework logs stop
// polluting stdout/stderr — which carry the command's ACTUAL output (the
// `llm` JSON, the terminal NDJSON protocol, task results). Cobra's own error
// output still goes to stderr, so genuine CLI errors stay visible.
//
//   - Opt out (keep logging on stderr, e.g. while debugging) with
//     KTW_LOG_STDERR=1.
//   - Tune the level with KTW_LOG_LEVEL (debug|info|warn|error; default info).
//
// Best-effort: any failure leaves the default stderr logging in place. Must be
// called as early as possible (before extension init logs anything) and after
// Env() is resolved.
func SetupFileLogging(name string) {
	if os.Getenv("KTW_LOG_STDERR") != "" {
		return
	}
	logDir := application.Env().LogDir()
	if logDir == "" {
		return
	}
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return
	}
	f, err := openRotatingLog(filepath.Join(logDir, name+".log"))
	if err != nil {
		return
	}
	level := slog.LevelInfo
	if lv := os.Getenv("KTW_LOG_LEVEL"); lv != "" {
		_ = level.UnmarshalText([]byte(lv))
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(f, &slog.HandlerOptions{Level: level})))
	log.SetOutput(f)
	// Intentionally retain f for the process lifetime: short-lived CLIs are
	// reclaimed by the OS on exit; long-lived servers keep it open, rotated.
}

// StampLogger returns base with the RUNTIME CONTEXT every line a universe core
// writes must carry (identity-and-index.md §1): the universe it runs in and
// the id of the project it serves. A bare `module.local` in a line is then
// resolvable by the line's own scope even after the file moves. Empty values
// are left out rather than written as "" — an absent stamp is visible, a blank
// one lies.
func StampLogger(base *slog.Logger, universe, projectID string) *slog.Logger {
	var attrs []any
	if universe != "" {
		attrs = append(attrs, "universe", universe)
	}
	if projectID != "" {
		attrs = append(attrs, "project_id", projectID)
	}
	if len(attrs) == 0 {
		return base
	}
	return base.With(attrs...)
}
