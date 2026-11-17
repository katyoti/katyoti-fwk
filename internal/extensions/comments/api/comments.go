// SPDX-License-Identifier: Apache-2.0

// Package api is the comments service contract. Anyone — a person, an agent, a
// module — may say something about an object, and answer what was said; a
// question filed as a request is answered here.
//
// A thread is the `comments` capability's owned named file `thread`
// (`comments/<id>.yaml` in the object's module), attached to every object: it
// travels with the object when a rename moves it and goes when the object is
// deleted. It is never in the object's file: a remark is not a change to the
// committed object (no diff, no merge conflict, no new hash).
package api

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// The capability and its file.
const (
	Capability = "comments"
	ThreadFile = "thread"
)

// CommentsServiceDef describes the service for KTSP publishing.
var CommentsServiceDef = meta.ServiceDef{
	Name:    "comments",
	Short:   "Say something about an object, answer a comment, read what was said",
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		"List": {Short: "The comments on an object, oldest first", ReadOnly: true},
		"Add":  meta.Method("Say something about an object, or answer one of its comments"),
	},
}

// CommentsService reads and adds comments.
type CommentsService interface {
	List(ctx context.Context, in ListInput) (ListOutput, error)
	Add(ctx context.Context, in AddInput) (AddOutput, error)
}

// Comment is one comment on an object.
type Comment struct {
	// ID is the comment's id within its object (c1, c2, …) — comments are only
	// ever added, so the sequence never reuses one.
	ID string       `json:"id" yaml:"id" jsonschema:"The comment's id within its object"`
	By cwmapi.Actor `json:"by" yaml:"by" jsonschema:"Who said it"`
	At string       `json:"at" yaml:"at" jsonschema:"When it was said (RFC 3339)"`
	// Text is what was said, in markdown.
	Text string `json:"text" yaml:"text" jsonschema:"What was said (markdown)"`
	// ReplyTo is the comment this one answers; empty = a new thread.
	ReplyTo string `json:"reply_to,omitempty" yaml:"reply_to,omitempty" jsonschema:"The comment this one answers; empty starts a thread"`
}

// Thread is what the thread file holds.
type Thread struct {
	Comments []Comment `json:"comments" yaml:"comments"`
}

// ListInput names the object.
type ListInput struct {
	ID string `json:"id" jsonschema:"Object id; required"`
}

// ListOutput is every comment, oldest first.
type ListOutput struct {
	Comments []Comment `json:"comments"`
}

// AddInput says something about an object, or answers a comment.
type AddInput struct {
	ID      string       `json:"id"                 jsonschema:"Object id; required"`
	Text    string       `json:"text"               jsonschema:"What to say (markdown); required"`
	By      cwmapi.Actor `json:"by"                 jsonschema:"Who says it - a person or an agent or a module; required"`
	ReplyTo string       `json:"reply_to,omitempty" jsonschema:"The comment this one answers; empty starts a thread"`
}

// AddOutput is the stored comment, with its id and time.
type AddOutput struct {
	Comment Comment `json:"comment"`
}
