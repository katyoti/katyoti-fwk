// SPDX-License-Identifier: Apache-2.0

package chatpanel

import (
	"encoding/json"
	"strings"

	"github.com/katyoti/katyoti-fwk/internal/core/panel"
	"github.com/katyoti/katyoti-fwk/internal/core/render"
)

// ArtifactsOf is every UI artifact a chat panel event shows, in its order:
// the artifacts of the conversation node a replace of it carries, and the
// artifact a replace of one artifact's node carries. A chat draws a run's
// artifacts from these alone: each is the artifact whole
// every patch applied, its status streaming until it is completed.
func ArtifactsOf(ev panel.Event) []Artifact {
	if ev.Kind != panel.KindOps {
		return nil
	}
	var out []Artifact
	for _, op := range ev.Ops {
		if op.Op != render.OpReplace || op.Node == nil {
			continue
		}
		switch {
		case op.Target == ArtifactsNode:
			kids, _ := op.Node["children"].([]any)
			for _, k := range kids {
				if a, ok := artifactOfNode(k); ok {
					out = append(out, a)
				}
			}
		case strings.HasPrefix(op.Target, NodeID("")):
			if a, ok := artifactOfNode(op.Node); ok {
				out = append(out, a)
			}
		}
	}
	return out
}

// Transient reports whether a chat panel event only shows an artifact
// still streaming: every op replaces the node of an artifact whose status
// is streaming. Such an event is sent to the chat, never journaled - the
// artifact's completion (or the next conversation replace) carries what it
// showed, so a reopened session draws the artifact as it ended.
func Transient(ev panel.Event) bool {
	if ev.Kind != panel.KindOps || len(ev.Ops) == 0 {
		return false
	}
	for _, op := range ev.Ops {
		if op.Op != render.OpReplace || !strings.HasPrefix(op.Target, NodeID("")) {
			return false
		}
		a, ok := artifactOfNode(op.Node)
		if !ok || a.Status != StatusStreaming {
			return false
		}
	}
	return true
}

// artifactOfNode decodes the artifact an artifact node carries; ok false for
// any other node.
func artifactOfNode(n any) (Artifact, bool) {
	var node map[string]any
	switch v := n.(type) {
	case render.Node:
		node = v
	case map[string]any:
		node = v
	default:
		return Artifact{}, false
	}
	id, _ := node["id"].(string)
	if !strings.HasPrefix(id, NodeID("")) || node["artifact"] == nil {
		return Artifact{}, false
	}
	raw, err := json.Marshal(node["artifact"])
	if err != nil {
		return Artifact{}, false
	}
	var a Artifact
	if json.Unmarshal(raw, &a) != nil || a.ID == "" {
		return Artifact{}, false
	}
	return a, true
}
