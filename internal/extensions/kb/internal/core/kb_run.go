// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/core/application"

	kbapi "github.com/katyoti/katyoti-fwk/internal/extensions/kb/api"
)

func (s *kbService) Run(ctx context.Context, in kbapi.RunInput) (kbapi.RunOutput, error) {
	if in.Path == "" {
		return kbapi.RunOutput{}, fmt.Errorf("path is required")
	}

	absPath, err := application.ProjectPath(in.Path)
	if err != nil {
		return kbapi.RunOutput{}, fmt.Errorf("resolve path: %w", err)
	}

	var out kbapi.RunOutput

	// Step 1: Sync
	syncOut, err := s.Sync(ctx, kbapi.SyncInput{Path: absPath})
	if err != nil {
		return kbapi.RunOutput{}, fmt.Errorf("sync: %w", err)
	}
	out.Sync = syncOut

	// Step 2: Extract
	extractOut, err := s.Extract(ctx, kbapi.ExtractInput{Path: absPath})
	if err != nil {
		return kbapi.RunOutput{}, fmt.Errorf("extract: %w", err)
	}
	out.Extract = extractOut

	// Step 3: Index
	indexOut, err := s.Index(ctx, kbapi.IndexInput{Path: absPath})
	if err != nil {
		return kbapi.RunOutput{}, fmt.Errorf("index: %w", err)
	}
	out.Index = indexOut

	return out, nil
}
