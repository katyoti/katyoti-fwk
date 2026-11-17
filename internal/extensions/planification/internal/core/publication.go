// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	actionsapi "github.com/katyoti/katyoti-fwk/internal/extensions/actions/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/planification/api"
)

// Publications implements api.PublicationService: a publication renders one
// view and hands its documents to the converter the target owns.
type Publications struct{}

// NewPublications returns the publication service.
func NewPublications() *Publications { return &Publications{} }

var _ api.PublicationService = (*Publications)(nil)

func publicationFields(o cwmapi.Object) api.PublicationFields {
	f, _, _ := cwmapi.DecodeCapability[api.PublicationFields](o, api.CapPublication)
	return f
}

func toPublication(o cwmapi.Object) api.Publication {
	return api.Publication{ID: o.ID, Name: o.Name, PublicationFields: publicationFields(o)}
}

// checkView refuses a view it cannot render, or a scope it needs and lacks.
func checkView(view, scope string) error {
	switch view {
	case api.ViewStream, api.ViewReleaseBoard, api.ViewReleasePlan:
		if scope == "" {
			return invalid("the %s view needs a scope", view)
		}
	case api.ViewChangeLog:
	default:
		return invalid("no view %q: stream or changelog or release-board or release-plan", view)
	}
	return nil
}

// Create records a publication in sys-planification.
func (p *Publications) Create(ctx context.Context, in api.CreatePublicationInput) (api.PublicationOutput, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" || in.Target == "" {
		return api.PublicationOutput{}, invalid("Create: name and target required")
	}
	if err := checkView(in.View, in.Scope); err != nil {
		return api.PublicationOutput{}, err
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.PublicationOutput{}, err
	}
	if _, _, err := ensureModule(ctx, cwm); err != nil {
		return api.PublicationOutput{}, err
	}
	o := cwmapi.Object{Type: api.PublicationType}
	o.Parent = api.SystemModule
	o.Name = name
	if err := o.SetCapability(api.CapPublication, api.PublicationFields{
		View: in.View, Scope: in.Scope, Lang: in.Lang, Converter: in.Target, Settings: in.Settings, OnRelease: in.OnRelease,
	}); err != nil {
		return api.PublicationOutput{}, err
	}
	out, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: api.SystemModule, Object: o})
	if err != nil {
		return api.PublicationOutput{}, err
	}
	return api.PublicationOutput{Publication: toPublication(out.Object), Path: out.Path}, nil
}

// Get reads one publication.
func (p *Publications) Get(ctx context.Context, in api.GetInput) (api.PublicationOutput, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.PublicationOutput{}, err
	}
	o, err := loadTyped(ctx, cwm, in.ID, api.PublicationType)
	if err != nil {
		return api.PublicationOutput{}, err
	}
	return api.PublicationOutput{Publication: toPublication(o)}, nil
}

// List lists the publications.
func (p *Publications) List(ctx context.Context, _ api.ListAllInput) (api.ListPublicationsOutput, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.ListPublicationsOutput{}, err
	}
	list, err := cwm.ObjectList(ctx, cwmapi.ObjectListInput{Type: api.PublicationType})
	if err != nil {
		return api.ListPublicationsOutput{}, err
	}
	out := api.ListPublicationsOutput{Publications: []api.Publication{}}
	for _, o := range list.Objects {
		out.Publications = append(out.Publications, toPublication(o))
	}
	sort.Slice(out.Publications, func(i, j int) bool { return out.Publications[i].ID < out.Publications[j].ID })
	return out, nil
}

// render renders a publication's view.
func (p *Publications) render(ctx context.Context, f api.PublicationFields) (api.DocumentsOutput, error) {
	v := NewViews()
	switch f.View {
	case api.ViewStream:
		return v.Stream(ctx, api.StreamInput{Name: f.Scope, Lang: f.Lang})
	case api.ViewChangeLog:
		return v.ChangeLog(ctx, api.ChangeLogInput{Module: f.Scope, Lang: f.Lang})
	case api.ViewReleaseBoard:
		return v.ReleaseBoard(ctx, api.ViewInput{ID: f.Scope, Lang: f.Lang})
	case api.ViewReleasePlan:
		return v.ReleasePlan(ctx, api.ViewInput{ID: f.Scope, Lang: f.Lang})
	}
	return api.DocumentsOutput{}, checkView(f.View, f.Scope)
}

// Preview renders a publication's documents without publishing them.
func (p *Publications) Preview(ctx context.Context, in api.GetInput) (api.DocumentsOutput, error) {
	got, err := p.Get(ctx, in)
	if err != nil {
		return api.DocumentsOutput{}, err
	}
	return p.render(ctx, got.Publication.PublicationFields)
}

// Publish renders the view, runs the converter with its settings plus the
// documents, and records the outcome on the publication — a failure too.
func (p *Publications) Publish(ctx context.Context, in api.GetInput) (api.PublishOutput, error) {
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.PublishOutput{}, err
	}
	o, err := loadTyped(ctx, cwm, in.ID, api.PublicationType)
	if err != nil {
		return api.PublishOutput{}, err
	}
	f := publicationFields(o)
	last := &api.Published{At: time.Now().UTC().Format(time.RFC3339)}
	runErr := func() error {
		docs, err := p.render(ctx, f)
		if err != nil {
			return err
		}
		last.Documents = len(docs.Documents)
		input := map[string]any{}
		for k, v := range f.Settings {
			input[k] = v
		}
		input[api.PublishDocumentsKey] = docs.Documents
		raw, err := json.Marshal(input)
		if err != nil {
			return err
		}
		actions, err := application.UseService[actionsapi.ActionService](ctx)
		if err != nil {
			return fmt.Errorf("no action service to run %s: %w", f.Converter, err)
		}
		res, err := actions.Run(ctx, actionsapi.RunInput{URI: f.Converter, Input: raw})
		if err != nil {
			return err
		}
		last.Result = string(res.Value)
		if len(last.Result) > 500 {
			last.Result = last.Result[:500] + "…"
		}
		return nil
	}()
	if runErr != nil {
		last.Error = runErr.Error()
	}
	f.Last = last
	if err := o.SetCapability(api.CapPublication, f); err != nil {
		return api.PublishOutput{}, err
	}
	saved, err := update(ctx, cwm, o)
	if err != nil {
		return api.PublishOutput{}, err
	}
	out := api.PublishOutput{Publication: toPublication(saved)}
	if runErr != nil {
		return out, fmt.Errorf("publications.Publish %s: %w", in.ID, runErr)
	}
	return out, nil
}

// OnReleased publishes again the publications marked on_release that show the
// release just shipped — their pages are out of date; the others are not
// touched. It returns the publications it published; one failing does not stop
// the others.
func (p *Publications) OnReleased(ctx context.Context, release string) ([]string, error) {
	list, err := p.List(ctx, api.ListAllInput{})
	if err != nil {
		return nil, err
	}
	rel, err := NewReleases().Get(ctx, api.GetInput{ID: release})
	if err != nil {
		return nil, err
	}
	tree := moduleTree(ctx)
	var done []string
	var errs []string
	for _, pub := range list.Publications {
		if !pub.OnRelease || !showsRelease(pub.PublicationFields, rel.Release, tree) {
			continue
		}
		if _, err := p.Publish(ctx, api.GetInput{ID: pub.ID}); err != nil {
			errs = append(errs, err.Error())
			continue
		}
		done = append(done, pub.ID)
	}
	if len(errs) > 0 {
		return done, fmt.Errorf("after %s was released: %s", release, strings.Join(errs, "; "))
	}
	return done, nil
}

// showsRelease is whether a publication's page shows a release: a release
// board or plan of that release or of the project release it ships in; the
// change log of its module, of a module containing it, or of the whole project;
// a stream (it spans every release).
func showsRelease(f api.PublicationFields, rel api.Release, parent map[string]string) bool {
	switch f.View {
	case api.ViewReleaseBoard, api.ViewReleasePlan:
		return f.Scope == rel.ID || (rel.ShipsIn != "" && f.Scope == rel.ShipsIn)
	case api.ViewChangeLog:
		if f.Scope == "" || rel.Project {
			return true
		}
		for m := rel.Module; m != ""; m = parent[m] {
			if m == f.Scope {
				return true
			}
		}
		return false
	}
	return true
}
