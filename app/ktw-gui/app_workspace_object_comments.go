// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"fmt"

	commentsapi "github.com/katyoti/katyoti-fwk/internal/extensions/comments/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
	"github.com/katyoti/katyoti-fwk/internal/kts/protocol/ktsp"
)

// WorkspaceObjectComment is one comment as the pane shows it.
type WorkspaceObjectComment struct {
	ID      string `json:"id"`
	By      string `json:"by"`
	ByKind  string `json:"byKind,omitempty"`
	At      string `json:"at"`
	Text    string `json:"text"`
	ReplyTo string `json:"replyTo,omitempty"`
}

// WorkspaceObjectComments is an object's comments, oldest first.
type WorkspaceObjectComments struct {
	Comments []WorkspaceObjectComment `json:"comments"`
	Error    string                   `json:"error,omitempty"`
}

func workspaceComment(c commentsapi.Comment) WorkspaceObjectComment {
	return WorkspaceObjectComment{ID: c.ID, By: c.By.ID, ByKind: c.By.Kind, At: c.At, Text: c.Text, ReplyTo: c.ReplyTo}
}

// WorkspaceObjectCommentList reads an object's comments.
//
// Wails-bound: window.go.main.App.WorkspaceObjectCommentList(universeName, objectID).
func (a *App) WorkspaceObjectCommentList(universeName, objectID string) WorkspaceObjectComments {
	sender, err := a.universeSender(universeName)
	if err != nil {
		return WorkspaceObjectComments{Error: fmt.Sprintf("Cannot reach universe %q: %v", universeName, err)}
	}
	return commentListVia(a.ctx, sender, objectID)
}

func commentListVia(ctx context.Context, sender ktsp.Sender, objectID string) WorkspaceObjectComments {
	raw, err := sender.Call(ctx, "ktw-agents:comments/list", commentsapi.ListInput{ID: objectID})
	if err != nil {
		return WorkspaceObjectComments{Error: err.Error()}
	}
	var out commentsapi.ListOutput
	if err := json.Unmarshal(raw, &out); err != nil {
		return WorkspaceObjectComments{Error: fmt.Sprintf("decode response: %v", err)}
	}
	res := WorkspaceObjectComments{Comments: []WorkspaceObjectComment{}}
	for _, c := range out.Comments {
		res.Comments = append(res.Comments, workspaceComment(c))
	}
	return res
}

// WorkspaceObjectCommentAdd says something about an object as the person using
// the GUI (their profile id), or answers a comment (replyTo).
//
// Wails-bound: window.go.main.App.WorkspaceObjectCommentAdd(universeName, objectID, text, replyTo, personID).
func (a *App) WorkspaceObjectCommentAdd(universeName, objectID, text, replyTo, personID string) WorkspaceObjectComments {
	sender, err := a.universeSender(universeName)
	if err != nil {
		return WorkspaceObjectComments{Error: fmt.Sprintf("Cannot reach universe %q: %v", universeName, err)}
	}
	return commentAddVia(a.ctx, sender, objectID, text, replyTo, personID)
}

func commentAddVia(ctx context.Context, sender ktsp.Sender, objectID, text, replyTo, personID string) WorkspaceObjectComments {
	if personID == "" {
		personID = "user"
	}
	in := commentsapi.AddInput{ID: objectID, Text: text, ReplyTo: replyTo, By: cwmapi.Actor{ID: personID, Kind: cwmapi.ActorPerson}}
	if _, err := sender.Call(ctx, "ktw-agents:comments/add", in); err != nil {
		return WorkspaceObjectComments{Error: err.Error()}
	}
	return commentListVia(ctx, sender, objectID)
}
