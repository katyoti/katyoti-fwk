// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	kbapi "github.com/katyoti/katyoti-fwk/internal/extensions/kb/api"
)

// kbService implements kbapi.KBService.
type kbService struct {
	cfg *KBServiceConfig
	// profiles resolves the EmbeddingProfiles service; through application
	// at call time unless a test sets it.
	profiles func(ctx context.Context) (kbapi.EmbeddingProfiles, error)
}

// NewKBService creates a new KBService from config.
func NewKBService(cfg *KBServiceConfig) (kbapi.KBService, error) {
	return &kbService{cfg: cfg, profiles: application.UseService[kbapi.EmbeddingProfiles]}, nil
}
