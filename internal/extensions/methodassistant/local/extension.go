// SPDX-License-Identifier: Apache-2.0

// Package local wires the method-assistant service into a binary.
package local

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/appcore/busclient"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/publish"
	"github.com/katyoti/katyoti-fwk/internal/extensions/methodassistant/api"
	macore "github.com/katyoti/katyoti-fwk/internal/extensions/methodassistant/internal/core"
	planapi "github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// RequestsHost is the provider whose `requests` service files and lists the
// work asked of a module (planification).
const RequestsHost = "ktw-agents"

// listRequests asks planification's List over the bus: the transport lives
// here, the core takes it as a function.
func listRequests(ctx context.Context, in planapi.ListInput) (planapi.ListOutput, error) {
	return busclient.Call[planapi.ListOutput](ctx, busclient.New(RequestsHost, planapi.RequestServiceDef), "List", in)
}

// readScope asks planification what a release holds (a project release: its
// module releases' and its increments' too) - or, for an increment, what it
// holds - over the bus.
func readScope(ctx context.Context, id string) ([]string, error) {
	out, err := busclient.Call[planapi.ScopeOutput](ctx, busclient.New(RequestsHost, planapi.ReleaseServiceDef), "Scope", planapi.GetInput{ID: id})
	if err != nil {
		var incErr error
		if out, incErr = busclient.Call[planapi.ScopeOutput](ctx, busclient.New(RequestsHost, planapi.IncrementServiceDef), "Scope", planapi.GetInput{ID: id}); incErr != nil {
			return nil, err
		}
	}
	ids := make([]string, 0, len(out.Targets))
	for _, t := range out.Targets {
		ids = append(ids, t.ID)
	}
	return ids, nil
}

// listReleases asks planification the open releases and increments of a
// module (of every module when empty) What's Next can be narrowed to.
func listReleases(ctx context.Context, module string) ([]macore.ReleaseChoice, error) {
	var out []macore.ReleaseChoice
	rels, err := busclient.Call[planapi.ListReleasesOutput](ctx, busclient.New(RequestsHost, planapi.ReleaseServiceDef), "List", planapi.ListInput{Module: module})
	if err != nil {
		return nil, err
	}
	for _, r := range rels.Releases {
		if r.State == planapi.ReleaseReleased || r.State == planapi.ReleaseDropped {
			continue
		}
		label := r.Name
		if r.Version != "" && r.Version != r.Name {
			label = r.Version + " - " + r.Name
		}
		out = append(out, macore.ReleaseChoice{ID: r.ID, Label: label})
	}
	incs, err := busclient.Call[planapi.ListIncrementsOutput](ctx, busclient.New(RequestsHost, planapi.IncrementServiceDef), "List", planapi.ListIncrementsInput{Module: module, State: planapi.IncrementOpen})
	if err == nil {
		for _, i := range incs.Increments {
			out = append(out, macore.ReleaseChoice{ID: i.ID, Label: i.ID + " - " + i.Title})
		}
	}
	return out, nil
}

// Extension publishes the MethodAssistantService (ktw-methods:method-assistant/<verb>).
// It needs a CWMService in the same process: every verb is an Object API call.
func Extension() application.Extension {
	return publish.PublishSimpleService[api.MethodAssistantService](
		func(context.Context) (api.MethodAssistantService, error) {
			return macore.New(listRequests).WithScope(readScope, listReleases), nil
		},
		publish.WithServiceDef(api.MethodAssistantServiceDef),
		publish.WithKTSPContainer(),
	)
}
