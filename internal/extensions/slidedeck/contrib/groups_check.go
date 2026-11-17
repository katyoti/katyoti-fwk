// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"fmt"
	"io/fs"
	"path"
	"strings"

	deckapi "github.com/katyoti/katyoti-fwk/internal/extensions/slidedeck/api"
	"gopkg.in/yaml.v3"
)

// CheckSlideGroups reads every group under root in fsys as the pack reader
// publishes it (slide-groups/), and names what is wrong: a file that is
// not a group, a name other than its file's, a slide that includes another
// group, and every lint error of its slides against the built-in layouts. A
// sidecar shipping groups runs it in a test: a build-time check of shipped
// files, with no catalog and no override, hence the embedded layouts
// (BuiltinLayouts).
func CheckSlideGroups(fsys fs.FS, root string) []string {
	entries, err := fs.ReadDir(fsys, root)
	if err != nil {
		return []string{err.Error()}
	}
	layouts, err := BuiltinLayouts()
	if err != nil {
		return []string{err.Error()}
	}
	var out []string
	for _, e := range entries {
		ext := strings.ToLower(path.Ext(e.Name()))
		if e.IsDir() || (ext != ".yaml" && ext != ".yml") {
			continue
		}
		raw, err := fs.ReadFile(fsys, path.Join(root, e.Name()))
		if err != nil {
			out = append(out, err.Error())
			continue
		}
		var g deckapi.SlideGroupBody
		dec := yaml.NewDecoder(strings.NewReader(string(raw)))
		dec.KnownFields(true)
		var y struct {
			Name        string `yaml:"name"`
			Description string `yaml:"description"`
			Slides      string `yaml:"slides"`
		}
		if err := dec.Decode(&y); err != nil {
			out = append(out, fmt.Sprintf("%s: %v", e.Name(), err))
			continue
		}
		g = deckapi.SlideGroupBody{Name: y.Name, Description: y.Description, Slides: y.Slides}
		if want := strings.TrimSuffix(e.Name(), path.Ext(e.Name())); g.Name != want {
			out = append(out, fmt.Sprintf("%s: name %q is not its file's name", e.Name(), g.Name))
		}
		if g.Description == "" || strings.TrimSpace(g.Slides) == "" {
			out = append(out, fmt.Sprintf("%s: a group has a description and slides", e.Name()))
		}
		md := "---\ntitle: \"" + g.Name + "\"\n---\n\n" + g.Slides
		if len(deckapi.Includes(deckapi.Parse(md))) > 0 {
			out = append(out, fmt.Sprintf("%s: a slide of a group includes another group", e.Name()))
		}
		for _, f := range deckapi.Lint(md, deckapi.Grammar{Layouts: layouts}) {
			if f.Severity == deckapi.SeverityError {
				out = append(out, fmt.Sprintf("%s: line %d: %s", e.Name(), f.Line-4, f.Message))
			}
		}
	}
	return out
}
