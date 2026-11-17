// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// templateRefPrefix is where ktw-docs publishes its Typst templates
// (embed/references/templates/<name>.typ).
const templateRefPrefix = "ktw-docs:/reference/templates/"

// resolveTemplate turns a template NAME ("report") into a file pandoc can
// read: the Typst template ktw-docs ships, opened through its own contribution
// and written to a temporary file. A path, or a name with an extension, is
// pandoc's to resolve and is returned as is. cleanup removes what was written.
func resolveTemplate(ctx context.Context, name, format string) (path string, cleanup func(), err error) {
	cleanup = func() {}
	if name == "" || strings.ContainsAny(name, `/\`) || filepath.Ext(name) != "" {
		return name, cleanup, nil
	}
	if _, statErr := os.Stat(name); statErr == nil {
		return name, cleanup, nil
	}
	if format != "pdf" {
		return "", cleanup, fmt.Errorf("template %q: the bundled templates are Typst layouts for PDF, not %s", name, format)
	}
	uri := templateRefPrefix + name + ".typ"
	layer, err := mbapi.Open(ctx, mbapi.Q{Reach: mbapi.ReachSelf}, uri, "")
	if err != nil {
		return "", cleanup, fmt.Errorf("template %q is neither a file nor a bundled template (%s): %w", name, uri, err)
	}
	dir, err := os.MkdirTemp("", "ktw-docs-template-*")
	if err != nil {
		_ = layer.Close()
		return "", cleanup, fmt.Errorf("template %q: %w", name, err)
	}
	path = filepath.Join(dir, name+".typ")
	if _, err := layer.WriteFile(path); err != nil {
		_ = os.RemoveAll(dir)
		return "", cleanup, fmt.Errorf("template %q: %w", name, err)
	}
	return path, func() { _ = os.RemoveAll(dir) }, nil
}
