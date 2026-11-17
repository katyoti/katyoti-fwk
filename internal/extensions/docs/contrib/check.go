// SPDX-License-Identifier: Apache-2.0

package contrib

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	docsapi "github.com/katyoti/katyoti-fwk/internal/extensions/docs/api"
	"gopkg.in/yaml.v3"
)

// CheckTemplates reads every template under root in fsys as ContributeTemplates
// publishes it, and names what is wrong: a file that is not a template (an
// unknown key included), a name other than its file's, a finding of
// docsapi.Check — includes resolved among the folder's templates and extra
// (templates in reach from elsewhere, such as ktw-docs' partial ones). A
// sidecar shipping templates runs it in a test.
func CheckTemplates(fsys fs.FS, root string, extra ...docsapi.ContentTemplate) []string {
	entries, err := fs.ReadDir(fsys, root)
	if err != nil {
		return []string{err.Error()}
	}
	var out []string
	all := map[string]docsapi.ContentTemplate{}
	for _, t := range extra {
		all[t.Name] = t
	}
	var names []string
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
		t, err := DecodeTemplate(raw)
		if err != nil {
			out = append(out, fmt.Sprintf("%s: %v", e.Name(), err))
			continue
		}
		if want := strings.TrimSuffix(e.Name(), path.Ext(e.Name())); t.Name != want {
			out = append(out, fmt.Sprintf("%s: name %q is not its file's name", e.Name(), t.Name))
		}
		all[t.Name] = t
		names = append(names, t.Name)
	}
	sort.Strings(names)
	lookup := func(n string) (docsapi.ContentTemplate, bool) { t, ok := all[n]; return t, ok }
	for _, n := range names {
		for _, f := range docsapi.Check(all[n], lookup) {
			out = append(out, n+": "+f)
		}
	}
	return out
}

// DecodeTemplate reads a template's YAML strictly: a key the type does not
// declare is an error, not a silent drop.
func DecodeTemplate(raw []byte) (docsapi.ContentTemplate, error) {
	var v any
	if err := yaml.Unmarshal(raw, &v); err != nil {
		return docsapi.ContentTemplate{}, err
	}
	j, err := json.Marshal(v)
	if err != nil {
		return docsapi.ContentTemplate{}, err
	}
	dec := json.NewDecoder(bytes.NewReader(j))
	dec.DisallowUnknownFields()
	var t docsapi.ContentTemplate
	if err := dec.Decode(&t); err != nil {
		return docsapi.ContentTemplate{}, err
	}
	return t, nil
}
