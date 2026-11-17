// SPDX-License-Identifier: Apache-2.0

// gen-data-type writes every generated system data-type into
// internal/core/contribution/embed/data-type/ from the Go type it is stated
// in (see internal/core/contribution/datatypegen), and the TypeScript of the
// ones the GUI reads into app/ktw-gui/frontend/src/generated/. Run through
// `go generate ./internal/core/contribution/`; a test in that package refuses
// a stale file.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/katyoti/katyoti-fwk/internal/core/contribution/datatypegen"
)

func main() {
	dir := filepath.Join("internal", "core", "contribution", "embed", "data-type")
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}
	tsDir := filepath.Join("app", "ktw-gui", "frontend", "src", "generated")
	if len(os.Args) > 2 {
		tsDir = os.Args[2]
	}
	for _, name := range datatypegen.Names() {
		out, err := datatypegen.Render(name)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		path := filepath.Join(dir, name+".yaml")
		if err := os.WriteFile(path, out, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println("wrote", path)
	}
	if err := os.MkdirAll(tsDir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	mt := filepath.Join(tsDir, datatypegen.MediaTypesTSName+".ts")
	if err := os.WriteFile(mt, datatypegen.RenderMediaTypesTS(), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("wrote", mt)
	for _, name := range datatypegen.TSNames() {
		out, err := datatypegen.RenderTS(name)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		path := filepath.Join(tsDir, name+".ts")
		if err := os.WriteFile(path, out, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println("wrote", path)
	}
}
