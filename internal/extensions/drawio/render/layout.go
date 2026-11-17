// SPDX-License-Identifier: Apache-2.0

package render

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/extensions/drawio/internal/archdiagram"
)

// LayoutELKGraph runs the engine's elkjs runner, with bun, on a raw ELK
// graph JSON document and returns the laid-out graph (nodes and edges
// annotated with computed x/y/width/height). It is the one execution path
// for ELK with bun: the drawio pipeline (when its context names no other
// runner) and DocService.LayoutDiagram both go through it.
func LayoutELKGraph(ctx context.Context, elkJSON []byte) ([]byte, error) {
	return archdiagram.LayoutELKGraphLocal(ctx, elkJSON)
}

// ELKRunner lays out a raw ELK graph JSON document: LayoutELKGraph, or a
// host's way to reach the process that owns bun.
type ELKRunner = archdiagram.ELKRunner

// WithELKRunner returns a context whose layouts (Emit, Relayout) go through
// run instead of bun in this process: a binary that leaves bun to ktw-docs
// sends the ELK graph to ktw-docs' docs/layout-diagram.
func WithELKRunner(ctx context.Context, run ELKRunner) context.Context {
	return archdiagram.WithELKRunner(ctx, run)
}

// HasELKRunner reports whether a context names its own ELK runner.
func HasELKRunner(ctx context.Context) bool { return archdiagram.ELKRunnerFrom(ctx) != nil }
