// SPDX-License-Identifier: Apache-2.0

package console

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
)

type outputFlagsKey struct{}

// withOutputFlags stores OutputFlags in context.
func withOutputFlags(ctx context.Context, flags *OutputFlags) context.Context {
	return context.WithValue(ctx, outputFlagsKey{}, flags)
}

// OutputFlagsFromContext retrieves OutputFlags from context, or nil.
func OutputFlagsFromContext(ctx context.Context) *OutputFlags {
	if f, ok := ctx.Value(outputFlagsKey{}).(*OutputFlags); ok {
		return f
	}
	return nil
}

// RenderConsoleOrFile renders output to stdout or to a file if --of is set.
// When --of is set, output is always JSON (UTF-8, no BOM) regardless of --out mode.
// This bypasses shell encoding issues on Windows.
func RenderConsoleOrFile(mode OutputMode, name string, v any, renderer any, ctx context.Context) error {
	flags := OutputFlagsFromContext(ctx)
	if flags != nil && flags.File != "" {
		return writeJSONFile(flags.File, v)
	}
	return RenderConsole(mode, name, v, renderer)
}

// writeJSONFile writes v as indented UTF-8 JSON directly to a file.
func writeJSONFile(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal JSON: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	fmt.Printf("Output written to %s\n", path)
	return nil
}
