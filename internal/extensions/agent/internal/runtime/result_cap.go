// SPDX-License-Identifier: Apache-2.0

package runtime

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"unicode/utf8"

	toolsapi "github.com/katyoti/katyoti-fwk/internal/extensions/tools/api"
)

// resultPreviewBytes is how much of a result over its limit the model sees.
const resultPreviewBytes = 16 * 1024

var unsafeFileChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// capToolResult keeps a tool's result within the tool's MaxResultSize (the
// default for a tool the loop does not know; 0 is unlimited): over it, the
// whole result is written to dir (when set) and the model gets its start,
// its size and what to do - one oversized answer never fills the model's
// context.
func capToolResult(tool toolsapi.Tool, toolID, result, dir string) string {
	limit := toolsapi.MaxResultSizeDefault
	if tool != nil {
		limit = tool.MaxResultSize()
	}
	if limit <= 0 || len(result) <= limit {
		return result
	}
	kept := ""
	if dir != "" {
		path := filepath.Join(dir, unsafeFileChars.ReplaceAllString(toolID, "_")+".txt")
		if err := os.MkdirAll(dir, 0o755); err == nil {
			if err := os.WriteFile(path, []byte(result), 0o644); err == nil {
				kept = fmt.Sprintf(" The whole result is kept in %s.", path)
			} else {
				slog.Warn("tool result: keeping the whole result failed", "path", path, "error", err)
			}
		}
	}
	n := min(resultPreviewBytes, limit)
	for n > 0 && !utf8.RuneStart(result[n]) {
		n--
	}
	return fmt.Sprintf("%s\n\n[Result cut: %d bytes, over this tool's limit of %d bytes; the first %d are above.%s Ask for less - a narrower selector or query, fewer fields - to get what you need.]",
		result[:n], len(result), limit, n, kept)
}
