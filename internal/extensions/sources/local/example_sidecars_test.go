// SPDX-License-Identifier: Apache-2.0

package local_test

import (
	"strings"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
)

// exampleSidecars are the made-up sidecars the source-model tests read, each
// as its embedded folder: a source-connector entry under
// embed/source-connectors/ and the data-types it names under
// embed/data-types/. ktw-example lists applications and their interfaces
// (a location, items, found by search), ktw-sample a tree of pages in
// spaces (scopes, fetch, no location), ktw-demo entities read from
// ktw-example's applications by their code.
var exampleSidecars = map[string]fstest.MapFS{
	"ktw-example": {
		"embed/source-connectors/example-application.yaml": {Data: []byte(`name: example-application
label: Applications Example
description: Des applications et leurs interfaces, copiées dans le module.
icon: building
color: teal
location: ktw-example:/data-type/example-application-location
sync: ktw-example:example/source-sync
test: ktw-example:example/source-test
browse: ktw-example:example/source-browse
search: ktw-example:example/source-search
propose: ktw-example:example/source-propose
items:
  application: ktw-example:/data-type/example-application
  interface: ktw-example:/data-type/example-interface
picker:
  unit: application
  layout: list
`)},
		"embed/data-types/example-application-location.dsl.yaml": {Data: []byte("name: example-application-location\n")},
		"embed/data-types/example-application.dsl.yaml": {Data: []byte(`name: example-application
types:
  ExampleApplication:
    type: object
    fields:
      name:
        type: string
      code:
        type: string
`)},
		"embed/data-types/example-interface.dsl.yaml": {Data: []byte("name: example-interface\n")},
	},
	"ktw-sample": {
		"embed/source-connectors/sample-pages.yaml": {Data: []byte(`name: sample-pages
label: Pages Sample
description: Les espaces et les pages, copiés dans le module en Markdown.
icon: file-text
color: blue
sync: ktw-sample:sample/source-sync
test: ktw-sample:sample/source-test
browse: ktw-sample:sample/source-browse
search: ktw-sample:sample/source-search
propose: ktw-sample:sample/source-propose
fetch: ktw-sample:sample/source-fetch
scopes:
  unit: espace
  list: ktw-sample:sample/source-scopes
  add: true
picker:
  unit: page
  layout: tree
`)},
	},
	"ktw-demo": {
		"embed/source-connectors/demo-entities.yaml": {Data: []byte(`name: demo-entities
label: Entités Demo
description: Les entités des applications prises, avec leurs relations, copiées dans le module.
icon: activity
color: orange
location: ktw-demo:/data-type/demo-entities-location
sync: ktw-demo:demo/source-sync
test: ktw-demo:demo/source-test
browse: ktw-demo:demo/source-browse
from: {connector: example-application, attr: code}
`)},
		"embed/data-types/demo-entities-location.dsl.yaml": {Data: []byte("name: demo-entities-location\n")},
	},
}

// exampleConnectors contributes a made-up sidecar's connectors the way the
// sidecar kit reads its pack: each entry of its embedded folder through
// the source-connector kind, under the sidecar's namespace.
func exampleConnectors(sidecar string) application.Extension {
	return contributeConnectors(sidecar, loader.NewEmbedFSLoader(exampleSidecars[sidecar], "."), "embed/source-connectors")
}

// exampleDataType answers where a made-up sidecar's data-type URI is
// authored - its embedded DSL file - and whether it is.
func exampleDataType(uri string) ([]byte, bool) {
	ns, name, _ := strings.Cut(uri, ":/data-type/")
	f, ok := exampleSidecars[ns]["embed/data-types/"+name+".dsl.yaml"]
	if !ok {
		return nil, false
	}
	return f.Data, true
}
