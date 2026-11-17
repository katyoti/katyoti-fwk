// SPDX-License-Identifier: Apache-2.0

package archdiagram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/scripts"
)

// elkPackageJSON is the package.json written alongside the elk-layout.ts script
// so that `bun install` can install elkjs in the temp dir.
var elkPackageJSON = []byte(`{"private":true,"dependencies":{"elkjs":"0.9.3"}}`)

// elkLayoutScript is the thin elkjs wrapper script, embedded as a Go constant.
// It reads ELK JSON from stdin, runs elk.layout(), writes result to stdout.
var elkLayoutScript = []byte(`import ELK from "elkjs";
const elk = new ELK({
  workerFactory: () => {
    return new Worker(
      new URL("./node_modules/elkjs/lib/elk-worker.js", import.meta.url).href
    );
  },
});
const input = await Bun.stdin.text();
const graph = JSON.parse(input);
const t0 = performance.now();
const result = await elk.layout(graph);
const elapsed = ((performance.now() - t0) / 1000).toFixed(2);
const nodeCount = result.children?.length ?? 0;
const edgeCount = result.edges?.length ?? 0;
console.error("ELK layout: " + nodeCount + " nodes, " + edgeCount + " edges, " + elapsed + "s");
elk.terminateWorker();
console.log(JSON.stringify(result));
`)

// RunELKLayout translates the pivot diagram to ELK format, invokes the
// elk-layout.ts wrapper script, and merges positions back into the diagram.
//
// The script is embedded as a Go constant — no external FS needed.
// The Go code handles all pivot↔ELK translation.
func RunELKLayout(ctx context.Context, diagram *ArchDiagram) error {
	// Translate pivot → ELK JSON
	elkGraph := pivotToELK(diagram)

	elkInput, err := json.Marshal(elkGraph)
	if err != nil {
		return fmt.Errorf("elk: marshal ELK graph: %w", err)
	}

	out, err := LayoutELKGraph(ctx, elkInput)
	if err != nil {
		return err
	}

	// Parse ELK output
	var elkResult elkGraph_t
	if err := json.Unmarshal(out, &elkResult); err != nil {
		return fmt.Errorf("elk: parse output: %w", err)
	}

	// Merge ELK positions back into pivot diagram
	mergeELKPositions(diagram, &elkResult)

	return nil
}

// ELKRunner lays out a raw ELK graph JSON document and returns the laid-out
// graph: the bun runner here, or a host's way to reach the process that owns
// bun (ktw-docs' docs/layout-diagram over the bus).
type ELKRunner func(ctx context.Context, elkJSON []byte) ([]byte, error)

type elkRunnerKey struct{}

// WithELKRunner returns a context whose layouts go through run instead of
// the local bun runner.
func WithELKRunner(ctx context.Context, run ELKRunner) context.Context {
	return context.WithValue(ctx, elkRunnerKey{}, run)
}

// ELKRunnerFrom is the runner a context carries, or nil.
func ELKRunnerFrom(ctx context.Context) ELKRunner {
	run, _ := ctx.Value(elkRunnerKey{}).(ELKRunner)
	return run
}

// LayoutELKGraph lays out a raw ELK graph JSON document and returns the
// laid-out graph JSON (nodes/edges annotated with computed x/y/width/height):
// through the context's runner when it carries one, else the local bun
// runner. Every layout of the pipeline goes through it.
func LayoutELKGraph(ctx context.Context, elkJSON []byte) ([]byte, error) {
	if run := ELKRunnerFrom(ctx); run != nil {
		return run(ctx, elkJSON)
	}
	return LayoutELKGraphLocal(ctx, elkJSON)
}

// LayoutELKGraphLocal runs the embedded elkjs wrapper with bun. This is the
// single execution path for the elk-layout script — the pipeline (when no
// runner is given) and DocService.LayoutDiagram (the public method) both
// route through it, so the script lives in exactly one place (the
// elkLayoutScript constant).
func LayoutELKGraphLocal(ctx context.Context, elkJSON []byte) ([]byte, error) {
	result, err := scripts.Run(ctx, scripts.RunInput{
		Name:      "elk-layout",
		Data:      elkLayoutScript,
		Extension: ".ts",
		CompanionFiles: map[string][]byte{
			"package.json": elkPackageJSON,
		},
		InstallDeps: true,
		Stdin:       bytes.NewReader(elkJSON),
		Timeout:     120 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("elk: %w", err)
	}
	if result.ExitCode != 0 {
		return nil, fmt.Errorf("elk: layout failed (exit %d): %s", result.ExitCode, result.Stderr)
	}
	return []byte(result.Stdout), nil
}

// --- Pivot → ELK translation ---

// elkGraph_t is the ELK JSON format (input and output).
type elkGraph_t struct {
	ID            string            `json:"id"`
	LayoutOptions map[string]string `json:"layoutOptions,omitempty"`
	Children      []elkNode_t       `json:"children,omitempty"`
	Edges         []elkEdge_t       `json:"edges,omitempty"`
}

type elkNode_t struct {
	ID            string            `json:"id"`
	Width         float64           `json:"width"`
	Height        float64           `json:"height"`
	X             float64           `json:"x,omitempty"`
	Y             float64           `json:"y,omitempty"`
	LayoutOptions map[string]string `json:"layoutOptions,omitempty"`
}

type elkEdge_t struct {
	ID       string         `json:"id"`
	Sources  []string       `json:"sources"`
	Targets  []string       `json:"targets"`
	Labels   []elkLabel_t   `json:"labels,omitempty"`
	Sections []elkSection_t `json:"sections,omitempty"`
}

// elkLabel_t is an edge label: its size in, its place out.
type elkLabel_t struct {
	ID string `json:"id"`
	// Text must be set: elkjs lays out no label without one.
	Text   string  `json:"text"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
	X      float64 `json:"x,omitempty"`
	Y      float64 `json:"y,omitempty"`
}

type elkSection_t struct {
	StartPoint elkPoint_t   `json:"startPoint"`
	EndPoint   elkPoint_t   `json:"endPoint"`
	BendPoints []elkPoint_t `json:"bendPoints,omitempty"`
}

type elkPoint_t struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// pivotToELK translates our pivot format to ELK's JSON format.
func pivotToELK(d *ArchDiagram) *elkGraph_t {
	layout := d.Layout
	if layout == nil {
		layout = DefaultLayout()
	}

	// Build zone order lookup
	zoneOrderMap := make(map[string]int)
	for _, z := range d.Zones {
		zoneOrderMap[z.ID] = z.Order
	}

	// Root layout options — start with defaults, then apply extras from preset
	opts := map[string]string{
		"elk.algorithm":        layout.Algorithm,
		"elk.direction":        layout.Direction,
		"elk.spacing.nodeNode": fmt.Sprintf("%.0f", layout.NodeSpacing),
		"elk.layered.spacing.nodeNodeBetweenLayers": fmt.Sprintf("%.0f", layout.LayerSpacing),
		"org.eclipse.elk.partitioning.activate":     "true",
		"elk.layered.crossingMinimization.strategy": "LAYER_SWEEP",
		"elk.edgeRouting":                           "ORTHOGONAL",
		"elk.layered.nodePlacement.strategy":        "NETWORK_SIMPLEX",
	}
	// Zones partition the whole graph, not each of its islands: ELK lays a
	// disconnected graph out component by component and packs them side by
	// side, each with its own partitions — the zones' columns interleave.
	if len(d.Zones) > 0 {
		opts["elk.separateConnectedComponents"] = "false"
	}
	// Merge extra options from layout preset (overrides defaults)
	for k, v := range layout.ExtraOptions {
		opts[k] = v
	}

	g := &elkGraph_t{
		ID:            "root",
		LayoutOptions: opts,
	}

	// A node outside every zone gets a partition of its own when its edges
	// say where it belongs: only into zones → before them, only out of
	// zones → after them. Otherwise it could land in a zone's rows and the
	// zone's frame, drawn around its members, would enclose it.
	outside := outsidePartitions(d, zoneOrderMap)

	// Translate nodes
	for _, n := range d.Nodes {
		elkNode := elkNode_t{
			ID:     n.ID,
			Width:  n.Width,
			Height: n.Height,
		}
		if order, ok := outside[n.ID]; ok {
			elkNode.LayoutOptions = map[string]string{
				"org.eclipse.elk.partitioning.partition": fmt.Sprintf("%d", order),
			}
		} else if order, ok := zoneOrderMap[n.Zone]; ok {
			elkNode.LayoutOptions = map[string]string{
				"org.eclipse.elk.partitioning.partition": fmt.Sprintf("%d", order),
			}
		}
		g.Children = append(g.Children, elkNode)
	}

	// Translate edges, with their labels' sizes so the layout keeps room
	// for them (ELK places a centre label between the layers it crosses).
	for _, e := range d.Edges {
		ee := elkEdge_t{
			ID:      e.ID,
			Sources: []string{e.Source},
			Targets: []string{e.Target},
		}
		if e.LabelW > 0 {
			ee.Labels = []elkLabel_t{{ID: e.ID + "-label", Text: e.ID, Width: e.LabelW, Height: e.LabelH}}
			g.LayoutOptions["elk.edgeLabels.placement"] = "CENTER"
		}
		g.Edges = append(g.Edges, ee)
	}

	return g
}

// outsidePartitions places the nodes outside every zone: a node whose
// edges all go into zones takes the partition before the first zone, one
// whose edges all come out of zones the partition after the last. A node
// with no zoned neighbour, or with both, is left alone.
func outsidePartitions(d *ArchDiagram, zoneOrder map[string]int) map[string]int {
	if len(zoneOrder) == 0 {
		return nil
	}
	lo, hi := math.MaxInt, math.MinInt
	for _, o := range zoneOrder {
		lo, hi = min(lo, o), max(hi, o)
	}
	zoneOf := map[string]string{}
	for _, n := range d.Nodes {
		zoneOf[n.ID] = n.Zone
	}
	into, from := map[string]bool{}, map[string]bool{}
	for _, e := range d.Edges {
		if zoneOf[e.Source] == "" && zoneOf[e.Target] != "" {
			into[e.Source] = true
		}
		if zoneOf[e.Target] == "" && zoneOf[e.Source] != "" {
			from[e.Target] = true
		}
	}
	out := map[string]int{}
	for _, n := range d.Nodes {
		switch {
		case n.Zone != "":
		case into[n.ID] && !from[n.ID]:
			out[n.ID] = lo - 1
		case from[n.ID] && !into[n.ID]:
			out[n.ID] = hi + 1
		}
	}
	return out
}

// --- ELK → Pivot merge ---

// mergeELKPositions merges ELK layout results back into the pivot diagram.
func mergeELKPositions(d *ArchDiagram, elk *elkGraph_t) {
	// Node positions
	posMap := make(map[string]*elkNode_t, len(elk.Children))
	for i := range elk.Children {
		posMap[elk.Children[i].ID] = &elk.Children[i]
	}
	for _, n := range d.Nodes {
		if en, ok := posMap[n.ID]; ok {
			n.X = en.X
			n.Y = en.Y
		}
	}

	// Edge bend points
	edgeMap := make(map[string]*elkEdge_t, len(elk.Edges))
	for i := range elk.Edges {
		edgeMap[elk.Edges[i].ID] = &elk.Edges[i]
	}
	for _, e := range d.Edges {
		if ee, ok := edgeMap[e.ID]; ok && len(ee.Sections) > 0 {
			sec := ee.Sections[0]
			var points []Point
			points = append(points, Point{X: sec.StartPoint.X, Y: sec.StartPoint.Y})
			for _, bp := range sec.BendPoints {
				points = append(points, Point{X: bp.X, Y: bp.Y})
			}
			points = append(points, Point{X: sec.EndPoint.X, Y: sec.EndPoint.Y})
			e.BendPoints = points
		}
		if ee, ok := edgeMap[e.ID]; ok && len(ee.Labels) > 0 && ee.Labels[0].Width > 0 {
			l := ee.Labels[0]
			e.LabelAt = &Point{X: l.X + l.Width/2, Y: l.Y + l.Height/2}
		}
	}

	// Zone bounds: computed from member nodes
	computeZoneBounds(d)
}

// computeZoneBounds computes the bounding box of each zone from its member nodes.
const zonePadding = 30.0

func computeZoneBounds(d *ArchDiagram) {
	for _, z := range d.Zones {
		minX, minY := math.Inf(1), math.Inf(1)
		maxX, maxY := math.Inf(-1), math.Inf(-1)
		hasMembers := false

		for _, n := range d.Nodes {
			if n.Zone != z.ID {
				continue
			}
			hasMembers = true
			if n.X < minX {
				minX = n.X
			}
			if n.Y < minY {
				minY = n.Y
			}
			if n.X+n.Width > maxX {
				maxX = n.X + n.Width
			}
			if n.Y+n.Height > maxY {
				maxY = n.Y + n.Height
			}
		}

		if !hasMembers {
			continue
		}

		z.Bounds = &Rect{
			X:      minX - zonePadding,
			Y:      minY - zonePadding,
			Width:  (maxX - minX) + zonePadding*2,
			Height: (maxY - minY) + zonePadding*2,
		}
	}
	// A zone holding zones is drawn around them too, innermost first.
	byID := map[string]*Zone{}
	for _, z := range d.Zones {
		byID[z.ID] = z
	}
	depth := func(z *Zone) int {
		k := 0
		for p := byID[z.Parent]; p != nil && k < len(d.Zones); p = byID[p.Parent] {
			k++
		}
		return k
	}
	inner := append([]*Zone(nil), d.Zones...)
	sort.SliceStable(inner, func(i, j int) bool { return depth(inner[i]) > depth(inner[j]) })
	for _, z := range inner {
		p := byID[z.Parent]
		if p == nil || z.Bounds == nil {
			continue
		}
		grown := Rect{X: z.Bounds.X - zonePadding, Y: z.Bounds.Y - zonePadding,
			Width: z.Bounds.Width + 2*zonePadding, Height: z.Bounds.Height + 2*zonePadding}
		if p.Bounds == nil {
			p.Bounds = &grown
			continue
		}
		x0, y0 := math.Min(p.Bounds.X, grown.X), math.Min(p.Bounds.Y, grown.Y)
		x1 := math.Max(p.Bounds.X+p.Bounds.Width, grown.X+grown.Width)
		y1 := math.Max(p.Bounds.Y+p.Bounds.Height, grown.Y+grown.Height)
		p.Bounds = &Rect{X: x0, Y: y0, Width: x1 - x0, Height: y1 - y0}
	}
}
