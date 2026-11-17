// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"

	docsapi "github.com/katyoti/katyoti-fwk/internal/extensions/docs/api"
	mbapi "github.com/katyoti/katyoti-fwk/internal/extensions/manifest/api"
)

// ContentTemplateObjectResult is a content-template object (a « trame ») as its
// Trame panel shows it: the template, and its parts in reading order with the
// section groups it includes expanded and marked. When an include cannot be
// expanded, Parts holds the template's own sections and ExpandError says why.
type ContentTemplateObjectResult struct {
	Template    docsapi.ContentTemplate `json:"template"`
	Parts       []docsapi.TemplatePart  `json:"parts"`
	ExpandError string                  `json:"expandError,omitempty"`
	Error       string                  `json:"error,omitempty"`
}

// ContentTemplateObject reads a content-template object for its Trame panel;
// its includes are found among the templates in reach (the project's first).
//
// Wails-bound: window.go.main.App.ContentTemplateObject(universeName, objectID).
func (a *App) ContentTemplateObject(universeName, objectID string) ContentTemplateObjectResult {
	sender, err := a.universeSender(universeName)
	if err != nil {
		return ContentTemplateObjectResult{Error: fmt.Sprintf("Cannot reach universe %q: %v", universeName, err)}
	}
	obj, _, err := readObjectVia(a.ctx, sender, objectID)
	if err != nil {
		return ContentTemplateObjectResult{Error: err.Error()}
	}
	t, err := docsapi.TemplateOfObject(obj)
	if err != nil {
		return ContentTemplateObjectResult{Error: err.Error()}
	}
	var inReach []mbapi.Attributed[docsapi.ContentTemplate]
	if cat, err := a.universeCatalog(universeName); err == nil {
		inReach, _ = docsapi.Templates(a.formOptionsCtx(universeName), mbapi.Q{Reach: mbapi.ReachUniverse, Catalog: cat})
	}
	return templateParts(t, inReach)
}

// templateParts expands t against the templates in reach; an include that
// cannot be expanded leaves t's own sections, with the reason.
func templateParts(t docsapi.ContentTemplate, inReach []mbapi.Attributed[docsapi.ContentTemplate]) ContentTemplateObjectResult {
	byName := map[string]docsapi.ContentTemplate{t.Name: t}
	for _, e := range inReach {
		if _, ok := byName[e.Body.Name]; !ok {
			byName[e.Body.Name] = e.Body
		}
	}
	out := ContentTemplateObjectResult{Template: t}
	parts, err := docsapi.Parts(t, func(n string) (docsapi.ContentTemplate, bool) { x, ok := byName[n]; return x, ok })
	if err != nil {
		out.ExpandError = err.Error()
		parts = nil
		for _, s := range t.Sections {
			parts = append(parts, docsapi.TemplatePart{Section: s})
		}
	}
	out.Parts = parts
	return out
}
