// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"log/slog"
	"slices"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	featuresapi "github.com/katyoti/katyoti-fwk/internal/extensions/features/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// A FEATURE IS BORN PROVEN — the work on a feature ends with the checks its
// request verified attached to the feature as its acceptance criteria (each
// one no criterion of it carries yet), through ktw-methods' features verbs:
// what proved the work goes on proving the feature, so an increment holding
// it can finish once they pass.

// attachCriterionURI attaches an acceptance criterion to a feature.
const attachCriterionURI = "ktw-methods:features/attach-criterion"

// proveFeature attaches to a feature the checks its request verified - each
// one no criterion of it already runs (probeKey: the same Go test however
// worded counts once), so a feature ended twice keeps one criterion per check.
// Nothing happens for an object that is no feature, a check that is an action
// URI (no shell command to probe), or with no action service.
func proveFeature(ctx context.Context, cwm cwmapi.CWMService, about string, f api.RequestFields) {
	o, err := loadObject(ctx, cwm, about)
	if err != nil || o.Type != featureType {
		return
	}
	actions, err := application.UseService[actionsapi.ActionService](ctx)
	if err != nil || actions == nil {
		return
	}
	have := criteriaProbes(ctx, cwm, about)
	for _, d := range f.Deliverables {
		check := strings.TrimSpace(d.Check)
		if !d.Verified || check == "" || have[probeKey(check)] || actionURI(check) {
			continue
		}
		in := featuresapi.AttachCriterionInput{Feature: about, Title: d.Title, Probe: check}
		if err := runAction(ctx, actions, attachCriterionURI, in, nil); err != nil {
			slog.Warn("planification: a verified check was not attached to its feature", "feature", about, "check", check, "error", err)
			continue
		}
		have[probeKey(check)] = true
	}
}

// criteriaProbes are the probe keys of the criteria a feature already carries:
// its children of the criterion type (what attach-criterion makes) and the
// criteria serving it.
func criteriaProbes(ctx context.Context, cwm cwmapi.CWMService, feature string) map[string]bool {
	out := map[string]bool{}
	for _, id := range criteriaServing(ctx, cwm, feature) {
		c, err := loadObject(ctx, cwm, id)
		if err != nil {
			continue
		}
		ck, _, _ := cwmapi.DecodeCapability[probeOf](c, capCheckable)
		if p := strings.TrimSpace(ck.Probe); p != "" {
			out[probeKey(p)] = true
		}
	}
	return out
}

// probeOf is what a criterion's checkable says it runs.
type probeOf struct {
	Probe string `json:"probe"`
}

// actionURI is a check naming an action (<sidecar>:<path>), not a command.
func actionURI(check string) bool {
	return !strings.ContainsAny(check, " \t") && strings.Contains(check, ":") && strings.Contains(check, "/")
}

// probeKey is what a probe runs, its wording aside: a Go test is its packages
// and its -run pattern (-count=1, -short and the other flags change how, not
// what); any other command is its words, spaces collapsed.
func probeKey(probe string) string {
	words := strings.Fields(probe)
	if len(words) < 2 || words[0] != "go" || words[1] != "test" {
		return strings.Join(words, " ")
	}
	var pkgs []string
	run := ""
	for i := 2; i < len(words); i++ {
		w := words[i]
		switch {
		case w == "-run" && i+1 < len(words):
			i++
			run = words[i]
		case strings.HasPrefix(w, "-run="):
			run = strings.TrimPrefix(w, "-run=")
		case strings.HasPrefix(w, "-"):
			if goTestValueFlags[w] {
				i++
			}
		default:
			pkgs = append(pkgs, strings.TrimSuffix(w, "/"))
		}
	}
	slices.Sort(pkgs)
	return "go test " + strings.Join(pkgs, " ") + " -run " + strings.Trim(run, `'"`)
}

// goTestValueFlags are the go test flags whose value is the next word.
var goTestValueFlags = map[string]bool{
	"-count": true, "-p": true, "-timeout": true, "-tags": true, "-parallel": true,
	"-cpu": true, "-bench": true, "-skip": true, "-coverprofile": true, "-covermode": true,
}
