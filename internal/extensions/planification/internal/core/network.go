// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

const eps = 1e-9

// schedule computes the network over the given nodes (After already limited
// to nodes of the network): each node's earliest and latest start and its
// slack, the critical path and the duration. It fails on a cycle, naming the
// nodes caught in it.
func schedule(nodes []api.NetNode) ([]api.NetNode, []string, float64, error) {
	idx := map[string]int{}
	for i, n := range nodes {
		idx[n.ID] = i
	}
	succ := map[string][]string{}
	indeg := map[string]int{}
	for _, n := range nodes {
		indeg[n.ID] += 0
		for _, p := range n.After {
			succ[p] = append(succ[p], n.ID)
			indeg[n.ID]++
		}
	}
	// Kahn's order, ties broken by id so the answer is stable.
	var ready, order []string
	for _, n := range nodes {
		if indeg[n.ID] == 0 {
			ready = append(ready, n.ID)
		}
	}
	for len(ready) > 0 {
		sort.Strings(ready)
		id := ready[0]
		ready = ready[1:]
		order = append(order, id)
		for _, s := range succ[id] {
			if indeg[s]--; indeg[s] == 0 {
				ready = append(ready, s)
			}
		}
	}
	if len(order) != len(nodes) {
		var caught []string
		for _, n := range nodes {
			if indeg[n.ID] > 0 {
				caught = append(caught, n.ID)
			}
		}
		sort.Strings(caught)
		return nil, nil, 0, invalid("the network has a cycle through %s", strings.Join(caught, ", "))
	}
	finish := map[string]float64{}
	duration := 0.0
	for _, id := range order {
		n := &nodes[idx[id]]
		for _, p := range n.After {
			n.EarliestStart = math.Max(n.EarliestStart, finish[p])
		}
		finish[id] = n.EarliestStart + n.Weight
		duration = math.Max(duration, finish[id])
	}
	latestFinish := map[string]float64{}
	for i := len(order) - 1; i >= 0; i-- {
		id := order[i]
		n := &nodes[idx[id]]
		lf := duration
		for _, s := range succ[id] {
			lf = math.Min(lf, nodes[idx[s]].LatestStart)
		}
		latestFinish[id] = lf
		n.LatestStart = lf - n.Weight
		n.Slack = n.LatestStart - n.EarliestStart
		if n.Slack < eps {
			n.Slack = 0
		}
		n.Critical = !n.Ended && n.Slack == 0
	}
	// One critical path: from a critical node starting at 0, follow a
	// critical successor starting where it finishes.
	var path []string
	if duration > eps {
		cur := ""
		for _, id := range order {
			if n := nodes[idx[id]]; n.Critical && n.EarliestStart < eps && n.Weight > eps {
				cur = id
				break
			}
		}
		for cur != "" {
			path = append(path, cur)
			end := finish[cur]
			next := ""
			for _, s := range succ[cur] {
				if n := nodes[idx[s]]; n.Critical && math.Abs(n.EarliestStart-end) < eps && (next == "" || s < next) {
					next = s
				}
			}
			cur = next
		}
	}
	return nodes, path, duration, nil
}

// Network answers a release's PERT network over its remaining work.
func (t *Planning) Network(ctx context.Context, in api.GetInput) (api.NetworkOutput, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.NetworkOutput{}, err
	}
	inc, err := loadRelease(ctx, cwm, in.ID)
	if err != nil {
		return api.NetworkOutput{}, err
	}
	objs, err := networkNodes(ctx, cwm, inc.ID)
	if err != nil {
		return api.NetworkOutput{}, err
	}
	members := map[string]cwmapi.Object{}
	for _, o := range objs {
		members[o.ID] = o
	}
	nodes := make([]api.NetNode, 0, len(objs))
	for _, o := range objs {
		n := api.NetNode{ID: o.ID, Type: o.Type, Request: hasRequest(o), Title: titleOf(o), State: nodeState(o), Ended: nodeEnded(o)}
		if hasRequest(o) && !n.Ended {
			n.Weight = requestWeight(o)
		}
		for _, d := range dependsOn(o) {
			if _, ok := members[d]; ok {
				n.After = append(n.After, d)
				continue
			}
			dep, err := loadNode(ctx, cwm, d)
			if err != nil {
				return api.NetworkOutput{}, fmt.Errorf("%s waits on %s: %w", o.ID, d, err)
			}
			if !nodeDone(dep) {
				n.Outside = append(n.Outside, d)
			}
		}
		nodes = append(nodes, n)
	}
	nodes, path, duration, err := schedule(nodes)
	if err != nil {
		return api.NetworkOutput{}, fmt.Errorf("planning.Network %s: %w", inc.ID, err)
	}
	out := api.NetworkOutput{Release: inc.ID, Runnable: []string{}, CriticalPath: path, Duration: duration}
	if out.CriticalPath == nil {
		out.CriticalPath = []string{}
	}
	for i := range nodes {
		n := &nodes[i]
		var waits []string
		for _, p := range n.After {
			if !nodeDone(members[p]) {
				waits = append(waits, p)
			}
		}
		waits = append(waits, n.Outside...)
		sort.Strings(waits)
		switch {
		case n.Type == api.StepType && !n.Ended:
			out.Steps = append(out.Steps, api.StepWait{ID: n.ID, WaitsOn: append([]string{}, waits...), Reachable: len(waits) == 0})
		case n.Request && !n.Ended && n.State != api.PlanBlocked && n.State != api.PlanSplit && len(waits) == 0:
			n.Runnable = true
			out.Runnable = append(out.Runnable, n.ID)
		}
	}
	out.Nodes = nodes
	return out, nil
}

// requestWeight is a request's remaining work: its estimate (1 when missing).
func requestWeight(o cwmapi.Object) float64 {
	if e := fieldsOf(o).Estimate; e > 0 {
		return e
	}
	return 1
}

// networkNodes is a release's steps and the requests targeting it, sorted by
// id. A target carrying no request — a goal nobody asked work on yet — is
// scope, not work: it is no node.
func networkNodes(ctx context.Context, cwm cwmapi.CWMService, rel string) ([]cwmapi.Object, error) {
	steps, err := childrenOf(ctx, cwm, rel, api.StepType)
	if err != nil {
		return nil, err
	}
	targets, err := workOf(ctx, cwm, rel)
	if err != nil {
		return nil, err
	}
	// A project release's plan is also its module releases': the work they
	// target crosses modules, and so does its critical path.
	modules, err := incoming(ctx, cwm, rel, api.EdgeShipsIn)
	if err != nil {
		return nil, err
	}
	for _, m := range modules {
		more, err := workOf(ctx, cwm, m)
		if err != nil {
			return nil, err
		}
		targets = append(targets, more...)
	}
	objs := slices.Clone(steps)
	for _, id := range targets {
		o, _, err := loadAny(ctx, cwm, id)
		if err != nil {
			return nil, err
		}
		if hasRequest(o) {
			objs = append(objs, o)
		}
	}
	sort.Slice(objs, func(i, j int) bool { return objs[i].ID < objs[j].ID })
	return objs, nil
}

// priorityRank orders priorities, P0 first; unset is the default priority.
func priorityRank(p string) int {
	if p == "" {
		p = api.DefaultPriority
	}
	if i := slices.Index(api.Priorities, p); i >= 0 {
		return i
	}
	return len(api.Priorities)
}
