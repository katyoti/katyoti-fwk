// SPDX-License-Identifier: Apache-2.0

// Package core keeps comments in the `comments` capability's thread file,
// through CWM's named-file verbs: CWM stores the file and keeps it with its
// object; this package reads and appends to it.
package core

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	"github.com/katyoti/katyoti-fwk/internal/extensions/comments/api"
	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// Service implements api.CommentsService over the CWMService in ctx.
type Service struct {
	mu sync.Mutex // an append is a read-modify-write of one file
}

// New returns the service.
func New() *Service { return &Service{} }

func cwmOf(ctx context.Context) (cwmapi.CWMService, error) {
	cwm, err := application.UseService[cwmapi.CWMService](ctx)
	if err != nil || cwm == nil {
		return nil, fmt.Errorf("comments: no CWMService: %v", err)
	}
	return cwm, nil
}

func read(ctx context.Context, cwm cwmapi.CWMService, id string) ([]api.Comment, error) {
	got, err := cwm.ObjectNamedFileRead(ctx, cwmapi.ObjectNamedFileReadInput{ID: id, Capability: api.Capability, Name: api.ThreadFile})
	if err != nil {
		return nil, err
	}
	if len(got.Content) == 0 {
		return []api.Comment{}, nil
	}
	var t api.Thread
	if err := marshal.Unmarshal(got.Content, marshal.YAML, &t); err != nil {
		return nil, fmt.Errorf("comments of %s: %w", id, err)
	}
	if t.Comments == nil {
		t.Comments = []api.Comment{}
	}
	return t.Comments, nil
}

// List returns an object's comments, oldest first.
func (s *Service) List(ctx context.Context, in api.ListInput) (api.ListOutput, error) {
	if in.ID == "" {
		return api.ListOutput{}, fmt.Errorf("%w: id required", cwmapi.ErrInvalidInput)
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.ListOutput{}, err
	}
	cs, err := read(ctx, cwm, in.ID)
	return api.ListOutput{Comments: cs}, err
}

// Add adds a comment — or, with ReplyTo, answers one.
func (s *Service) Add(ctx context.Context, in api.AddInput) (api.AddOutput, error) {
	if strings.TrimSpace(in.Text) == "" {
		return api.AddOutput{}, fmt.Errorf("%w: a comment says something — text required", cwmapi.ErrInvalidInput)
	}
	if in.By.ID == "" {
		return api.AddOutput{}, fmt.Errorf("%w: who says it is recorded — by required", cwmapi.ErrInvalidInput)
	}
	switch in.By.Kind {
	case cwmapi.ActorPerson, cwmapi.ActorAgent, "module":
	default:
		return api.AddOutput{}, fmt.Errorf("%w: by.kind %q — a comment is said by a person, an agent or a module", cwmapi.ErrInvalidInput, in.By.Kind)
	}
	cwm, err := cwmOf(ctx)
	if err != nil {
		return api.AddOutput{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cs, err := read(ctx, cwm, in.ID)
	if err != nil {
		return api.AddOutput{}, err
	}
	if in.ReplyTo != "" && !has(cs, in.ReplyTo) {
		return api.AddOutput{}, fmt.Errorf("%w: it answers comment %q, which %s does not have", cwmapi.ErrInvalidInput, in.ReplyTo, in.ID)
	}
	c := api.Comment{ID: fmt.Sprintf("c%d", len(cs)+1), By: in.By, At: time.Now().UTC().Format(time.RFC3339), Text: in.Text, ReplyTo: in.ReplyTo}
	data, err := marshal.Marshal(api.Thread{Comments: append(cs, c)}, marshal.YAML)
	if err != nil {
		return api.AddOutput{}, err
	}
	if _, err := cwm.ObjectNamedFileWrite(ctx, cwmapi.ObjectNamedFileWriteInput{ID: in.ID, Capability: api.Capability, Name: api.ThreadFile, Content: data}); err != nil {
		return api.AddOutput{}, fmt.Errorf("comments of %s: %w", in.ID, err)
	}
	return api.AddOutput{Comment: c}, nil
}

func has(cs []api.Comment, id string) bool {
	for _, c := range cs {
		if c.ID == id {
			return true
		}
	}
	return false
}
