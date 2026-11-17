// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"

	kbapi "github.com/katyoti/katyoti-fwk/internal/extensions/kb/api"
)

// EmbeddingCheck reports whether an embedding profile is ready to embed,
// without embedding: a profile whose key variable is unset is an error
// naming the variable, its check still returned.
func (s *kbService) EmbeddingCheck(ctx context.Context, in kbapi.EmbeddingCheckInput) (kbapi.EmbeddingProfileCheck, error) {
	ps, err := s.profiles(ctx)
	if err != nil {
		return kbapi.EmbeddingProfileCheck{}, fmt.Errorf("embedding profiles: %w", err)
	}
	c, err := ps.Check(in.Profile)
	if err != nil {
		return c, err
	}
	if !c.KeySet {
		return c, fmt.Errorf("embedding profile %q (%s): the environment variable %s holding its key is not set", c.Profile, c.Type, c.KeyEnv)
	}
	return c, nil
}
