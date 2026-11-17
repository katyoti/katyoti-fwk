// SPDX-License-Identifier: Apache-2.0

// Package local wires the panel server: Extension publishes
// its reads (PanelService) as the binary's KTSP slot, and Serve starts it on
// the bus - the universe's hub runs it, the one writer of
// <DataDir>/panels/. A kept panel's outcome goes to its
// object through CWM: a document in the object's module, attached to it.
package local

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	"github.com/katyoti/katyoti-fwk/internal/core/panel"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
	panelapi "github.com/katyoti/katyoti-fwk/internal/extensions/panelserver/api"
	panelcore "github.com/katyoti/katyoti-fwk/internal/extensions/panelserver/internal/core"
)

// DataSubDir is the server's folder under the universe's data folder.
const DataSubDir = "panels"

// Extension publishes PanelService, the panel server's reads, over KTSP.
// The KTSP container publishes every method of the value it is given, so the
// service is a value whose method set is PanelService's alone (published):
// the server's own methods - Take, Input, Prune, Follow - stay off the bus.
// In process, Follower gives the server's follow.
func Extension() application.Extension {
	return func(init *application.ApplicationInit) {
		init.AddExtension(appcore.PublishSimpleService[panelapi.PanelService](
			newService,
			appcore.WithServiceDef(panelapi.PanelServiceDef),
			appcore.WithKTSPContainer(),
		))
	}
}

// published is the panel server as the bus sees it: PanelService's methods
// and no other.
type published struct {
	panelapi.PanelService
	srv *panelcore.Server
}

func newService(ctx context.Context) (panelapi.PanelService, error) {
	srv, err := panelcore.New(panelcore.Options{
		Dir:       application.Env().GetDataSubDir(DataSubDir),
		Retention: typeRetention,
		KeepOf:    typeKeep,
		Keep:      keepOnObject,
		// The page templates a panel type's page names.
		TemplateBody: catalogBody,
	})
	if err != nil {
		return nil, err
	}
	return published{PanelService: srv, srv: srv}, nil
}

// typeDecl reads a panel type from its panel-type contribution
// (panel.TypeDecl); ok false when the type is not known or does not read.
func typeDecl(ctx context.Context, typeURI string) (panel.TypeDecl, bool) {
	if typeURI == "" {
		return panel.TypeDecl{}, false
	}
	cat, err := application.UseService[mbapi.Catalog](ctx)
	if err != nil || cat == nil {
		return panel.TypeDecl{}, false
	}
	body, err := cat.GetBody(ctx, typeURI)
	if err != nil || len(body) == 0 {
		return panel.TypeDecl{}, false
	}
	var decl panel.TypeDecl
	if err := config.DecodeBytes(body, marshal.YAML, &decl); err != nil {
		slog.Warn("panel server: a panel type does not read", "type", typeURI, "err", err)
		return panel.TypeDecl{}, false
	}
	return decl, true
}

// catalogBody is a contribution's body from the universe's catalog: a page
// template's, read by its URI.
func catalogBody(ctx context.Context, uri string) ([]byte, error) {
	cat, err := application.UseService[mbapi.Catalog](ctx)
	if err != nil || cat == nil {
		return nil, fmt.Errorf("no catalog: %v", err)
	}
	return cat.GetBody(ctx, uri)
}

// typeRetention is a panel type's retention; 0 - as ended runs - when the
// type is not known or says nothing.
func typeRetention(ctx context.Context, typeURI string) time.Duration {
	decl, ok := typeDecl(ctx, typeURI)
	if !ok {
		return 0
	}
	d, err := decl.RetentionDuration()
	if err != nil {
		return 0
	}
	return d
}

// typeKeep is what a panel type keeps once a panel ends; none when the type
// is not known or says nothing.
func typeKeep(ctx context.Context, typeURI string) string {
	decl, ok := typeDecl(ctx, typeURI)
	if !ok {
		return panel.KeepNone
	}
	return decl.KeepOrDefault()
}

// keepOnObject writes a kept panel's outcome as a document in its object's
// module (panels/<panel id>.md) and attaches it to the object, through CWM;
// it answers the document's module-relative path.
func keepOnObject(ctx context.Context, object string, o panelapi.Outcome) (string, error) {
	doc, err := panelcore.OutcomeDocument(o)
	if err != nil {
		return "", err
	}
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return "", fmt.Errorf("panel server: keep %s: %w", o.Snapshot.Panel, err)
	}
	path := panelcore.DocumentPath(o.Snapshot.Panel)
	w, err := cwm.ObjectFileWrite(ctx, cwmapi.ObjectFileWriteInput{ID: object, Path: path, Content: doc})
	if err != nil {
		return "", fmt.Errorf("panel server: keep %s on %s: %w", o.Snapshot.Panel, object, err)
	}
	if _, err := cwm.EdgeCreate(ctx, cwmapi.EdgeCreateInput{From: object, To: w.ProjectPath, Kind: cwmapi.EdgeKindAttaches}); err != nil {
		return "", fmt.Errorf("panel server: attach %s to %s: %w", w.ProjectPath, object, err)
	}
	return w.Path, nil
}

// Serve starts the application's panel server on bus, subscribed to every
// panel subject of tenant and universe; stop ends it.
func Serve(ctx context.Context, bus mbapi.MessageBus, tenant, universe string) (stop func(), err error) {
	svc, err := application.UseService[panelapi.PanelService](ctx)
	if err != nil {
		return nil, fmt.Errorf("panel server: %w", err)
	}
	srv, err := serverOf(svc)
	if err != nil {
		return nil, err
	}
	return srv.Start(ctx, bus, tenant, universe)
}

// Follower is the application's panel server's in-process follow.
func Follower(ctx context.Context) (panelapi.PanelFollow, error) {
	svc, err := application.UseService[panelapi.PanelService](ctx)
	if err != nil {
		return nil, fmt.Errorf("panel server: %w", err)
	}
	return serverOf(svc)
}

func serverOf(svc panelapi.PanelService) (*panelcore.Server, error) {
	p, ok := svc.(published)
	if !ok || p.srv == nil {
		return nil, fmt.Errorf("panel server: PanelService is no panel server here (%T)", svc)
	}
	return p.srv, nil
}
