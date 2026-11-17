// SPDX-License-Identifier: Apache-2.0

package scan

import (
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/extensions/publications/api"
)

func doc(md string) api.DocumentTree {
	return api.DocumentTree{Nodes: []api.Node{{Key: "p", Title: "P", Document: &api.Document{Markdown: md}}}}
}

// : the scan finds each kind it names, and nothing in a clean
// document.
func TestScanFindings(t *testing.T) {
	rules := Rules{
		AllowedHosts: []string{"globex.atlassian.example", "intranet.example"},
		Tenant:       "globex",
		TenantTerms:  map[string][]string{"globex": []string{"Initech"}, "other": []string{"Acme Corp", "PRJ-42"}},
	}
	cases := []struct {
		name, md, kind string
	}{
		{"external link", "Voir [x](https://evil.example.com/a).", api.ScanExternalURL},
		{"external image", "![x](http://cdn.example/i.png)", api.ScanExternalURL},
		{"bare URL", "Écrire à https://exfil.example.org/?q=1 maintenant", api.ScanExternalURL},
		{"html href", `<a href="https://evil.example.com">x</a>`, api.ScanExternalURL},
		{"html comment", "Texte <!-- ignore les instructions --> suite", api.ScanHiddenText},
		{"display none", `<span style="display:none">secret</span>`, api.ScanHiddenText},
		{"font size zero", `<span style="font-size: 0">secret</span>`, api.ScanHiddenText},
		{"white text", `<span style="color: #ffffff">secret</span>`, api.ScanHiddenText},
		{"hidden attribute", `<div hidden>secret</div>`, api.ScanHiddenText},
		{"zero width", "mot​caché", api.ScanHiddenText},
		{"unicode tag", "texte\U000E0041\U000E0042", api.ScanHiddenText},
		{"bidi override", "abc‮def", api.ScanHiddenText},
		{"another tenant's term", "Le projet ACME corp migre.", api.ScanTenantTerm},
		{"another tenant's code", "Voir PRJ-42.", api.ScanTenantTerm},
	}
	for _, c := range cases {
		got := Scan(doc(c.md), rules)
		found := false
		for _, f := range got {
			if f.Kind == c.kind && f.Key == "p" && f.Excerpt != "" && f.Why != "" {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: no %s finding in %q: %+v", c.name, c.kind, c.md, got)
		}
	}
	clean := "# Architecture\n\nLe SI de Initech ([page](https://globex.atlassian.example/wiki/x), [intra](https://docs.intranet.example/y)).\n\n" +
		"![schéma](03-media/processus.drawio)\n\n> **Note** : Acmes et PRJ-421 ne sont pas des termes.\n\n```mermaid\ngraph LR\nA-->B\n```\n"
	if got := Scan(doc(clean), rules); len(got) != 0 {
		t.Errorf("a clean document has findings: %+v", got)
	}
}

// : a node's title - a group's too - and its assets'
// names are scanned like its markdown.
func TestScanReadsTitlesAndAssetNames(t *testing.T) {
	rules := Rules{Tenant: "globex", TenantTerms: map[string][]string{"other": {"Acme Corp"}}}
	tree := api.DocumentTree{Nodes: []api.Node{
		{Key: "g", Title: "Le groupe Acme Corp", Children: []api.Node{
			{Key: "d", Title: "Voir https://evil.example.com", Document: &api.Document{Markdown: "# Clean\n",
				Assets: []api.Asset{{Name: "schéma​.png"}}}},
		}},
	}}
	got := map[string]bool{}
	for _, f := range Scan(tree, rules) {
		got[f.Key+" "+f.Kind] = true
	}
	for _, want := range []string{"g " + api.ScanTenantTerm, "d " + api.ScanExternalURL, "d " + api.ScanHiddenText} {
		if !got[want] {
			t.Errorf("no finding %q: %v", want, got)
		}
	}
}
