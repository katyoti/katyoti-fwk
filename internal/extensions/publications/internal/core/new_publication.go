// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/publications/api"
)

// NewPublicationPage is the publication type's create page under its holder
// (mock-up v6 view 29): « Où », one choice card per
// contributed publication connector - choosing one names the publication
// after it -, « Nom », and « Créer », disabled until both are set, which
// makes the holder's publication and opens it; « Annuler » closes the dialog.
func (s *Service) NewPublicationPage(ctx context.Context, in api.NewPublicationPageInput) (api.PublicationPageOutput, error) {
	parent := strings.TrimSpace(in.Parent)
	if parent == "" {
		return api.PublicationPageOutput{}, fmt.Errorf("%w: publications.NewPublicationPage: parent required", cwmapi.ErrInvalidInput)
	}
	list, err := s.Connectors(ctx, api.ConnectorsInput{})
	if err != nil {
		return api.PublicationPageOutput{}, err
	}
	return api.PublicationPageOutput{DSLTree: newPublicationTree(parent, list.Connectors), Data: map[string]any{}, Title: "Nouvelle publication"}, nil
}

// newPublicationTree is the create page from the connectors contributed.
func newPublicationTree(parent string, connectors []api.ConnectorDecl) render.Node {
	cards := make([]render.ChoiceCard, 0, len(connectors))
	for _, c := range connectors {
		label := connectorLabel(c)
		cards = append(cards, render.ChoiceCard{Value: c.Name, Label: label, Description: c.Description, Icon: c.Icon,
			Sets: map[string]string{"name": label}})
	}
	children := []any{
		render.Node{"kind": "text", "content": "Où", "size": "sm", "weight": 600},
		render.ChoiceCards("connector", "Où", cards...).With("id", "new-publication-connectors").With("cols", 1),
	}
	if len(cards) == 0 {
		children = append(children, render.Node{"kind": "text", "content": "Aucun connecteur de publication n'est installé.", "size": "sm", "color": "dimmed"})
	}
	children = append(children,
		render.TextField("name", "Nom", "").With("placeholder", "Choisissez d'abord où elle publie").With("required", true),
		render.Node{"kind": "text", "content": "La page parente se choisit ensuite dans son onglet Destination.", "size": "sm", "color": "dimmed"},
		render.HBox(render.Button("Annuler", render.CloseHere("Annuler")).
			With("id", "new-publication-cancel").With("variant", "default").With("size", "sm"),
			render.Button("Créer", render.Action{Label: "Créer", Icon: "plus", URI: api.PublicationNewURI,
				Args: map[string]any{"parent": parent, "connector": "${selection.connector}", "name": "${selection.name}"}}).
				With("id", "new-publication-create").With("variant", "filled").With("size", "sm").
				Requires("Choisissez où elle publie et donnez-lui un nom.", "connector", "name")).With("justify", "flex-end"),
	)
	return render.Node{"kind": "vbox", "gap": "sm", "children": children}
}

func connectorLabel(c api.ConnectorDecl) string {
	if strings.TrimSpace(c.Label) != "" {
		return c.Label
	}
	return c.Name
}

// PublicationNew creates the holder's publication through a contributed
// connector, in the holder's module, and opens it.
func (s *Service) PublicationNew(ctx context.Context, in api.PublicationNewInput) (api.PublicationNewOutput, error) {
	parent, name := strings.TrimSpace(in.Parent), strings.TrimSpace(in.Name)
	if parent == "" {
		return api.PublicationNewOutput{}, fmt.Errorf("%w: publications.PublicationNew: parent required", cwmapi.ErrInvalidInput)
	}
	c, err := s.connector(ctx, strings.TrimSpace(in.Connector))
	if err != nil {
		return api.PublicationNewOutput{Message: err.Error()}, nil
	}
	if name == "" {
		name = connectorLabel(c)
	}
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil {
		return api.PublicationNewOutput{}, fmt.Errorf("publications: resolve CWMService: %w", err)
	}
	module, err := cwmapi.ModuleOfObjectID(parent)
	if err != nil {
		return api.PublicationNewOutput{}, err
	}
	if _, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: parent}); err != nil {
		return api.PublicationNewOutput{}, fmt.Errorf("publications: the holder %s: %w", parent, err)
	}
	local := "pb-" + c.Name
	for i := 2; ; i++ {
		if _, err := cwm.ObjectGet(ctx, cwmapi.ObjectGetInput{Module: module, ID: cwmapi.QualifyObjectID(module, local)}); err != nil {
			break
		}
		local = fmt.Sprintf("pb-%s-%d", c.Name, i)
	}
	fields := api.PublicationFields{Object: parent, Connector: c.Name}
	if err := fields.Check(); err != nil {
		return api.PublicationNewOutput{}, err
	}
	obj := cwmapi.NewContainerObject(module, local, api.PublicationType, parent, name, "")
	if err := obj.SetCapability(api.CapPublication, fields); err != nil {
		return api.PublicationNewOutput{}, err
	}
	made, err := cwm.ObjectCreate(ctx, cwmapi.ObjectCreateInput{Module: module, Object: obj})
	if err != nil {
		return api.PublicationNewOutput{}, fmt.Errorf("publications: create a publication of %s: %w", parent, err)
	}
	return api.PublicationNewOutput{OK: true, Publication: made.Object.ID,
		Open:    &render.Navigate{Object: made.Object.ID},
		Message: "Publication créée : choisissez sa destination.",
	}, nil
}
