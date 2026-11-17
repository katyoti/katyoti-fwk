// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// THE PROJECT OF EACH OBJECT. A universe serving several projects
// - two project folders of one repository, or two repositories - answers
// through cwm's store set (storeset.Served), which names the key of
// the project holding each object: a list or a search answers its `objects`
// as one store does and, beside them, `projects` - each object's key, in the
// same order (cwmapi.ObjectListOutput / ObjectSearchOutput); a tree node or an
// object's detail carries `project` beside its object. One store answers no
// key: every reader here gives "" then, and the screens draw no project mark.

// projectOf is the `project` key an answer carries at its top; "" when none.
func projectOf(raw []byte) string {
	var p struct {
		Project string `json:"project"`
	}
	if json.Unmarshal(raw, &p) != nil {
		return ""
	}
	return p.Project
}

// treeProjects is each tree node's project key, depth first - the order
// objectsTreeVia flattens the tree in; "" for a node naming none.
func treeProjects(raw []byte) []string {
	type node struct {
		Project  string `json:"project"`
		Children []node `json:"children"`
	}
	var resp struct {
		Roots []node `json:"roots"`
	}
	if json.Unmarshal(raw, &resp) != nil {
		return nil
	}
	var out []string
	var walk func([]node, string)
	walk = func(nodes []node, parent string) {
		for _, n := range nodes {
			p := n.Project
			if p == "" {
				p = parent // a node below a project's root is that project's
			}
			out = append(out, p)
			walk(n.Children, p)
		}
	}
	walk(resp.Roots, "")
	return out
}

// locatedObject is one listed or searched object and the project holding it.
type locatedObject struct {
	Project string
	Object  cwmapi.Object
}

// locatedObjects reads an answer's `objects` with each one's project from the
// parallel `projects` list. One store answers no list: no object is marked.
// A list whose length is not the objects' cannot be matched by position, so
// it marks none either.
func locatedObjects(raw []byte) ([]locatedObject, error) {
	var resp struct {
		Objects  []cwmapi.Object `json:"objects"`
		Projects []string        `json:"projects"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, err
	}
	marked := len(resp.Projects) == len(resp.Objects)
	out := make([]locatedObject, 0, len(resp.Objects))
	for i, o := range resp.Objects {
		l := locatedObject{Object: o}
		if marked {
			l.Project = resp.Projects[i]
		}
		out = append(out, l)
	}
	return out, nil
}
