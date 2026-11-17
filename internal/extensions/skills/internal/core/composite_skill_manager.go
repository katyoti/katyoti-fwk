// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"errors"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/extensions/skills/api"
)

// compositeSkillManager aggregates multiple SkillManager sources for
// the in-process accessors (GetSkill, GetSkillRequires). Each call
// tries every source in order and returns the first success.
//
// Enumeration + L1 instructions live on `mbapi.Catalog` (step 13 of
// `bus-split-aggregator-to-catalog`); the RPC methods GetReference +
// RunScript were dropped 2026-05-24 — references are first-class
// Contributions via ResourceService.Get; standalone scripts use
// the `<provider>:scripts/<name>` slot.
type compositeSkillManager struct {
	sources []api.SkillManager
}

// NewCompositeSkillManager creates a SkillManager that queries multiple sources.
// The first source (typically the local manager) has priority.
func NewCompositeSkillManager(sources ...api.SkillManager) api.SkillManager {
	return &compositeSkillManager{sources: sources}
}

func (c *compositeSkillManager) GetSkill(name string) (api.Skill, error) {
	for _, src := range c.sources {
		skill, err := src.GetSkill(name)
		if err == nil {
			return skill, nil
		}
	}
	return nil, fmt.Errorf("skill %q not found in any source", name)
}

func (c *compositeSkillManager) GetSkillRequires(name string) []string {
	for _, src := range c.sources {
		if reqs := src.GetSkillRequires(name); reqs != nil {
			return reqs
		}
	}
	return nil
}

// Reload fans out to every source. Aggregates errors — one source's
// failure does NOT stop the others from refreshing (a degraded source
// should not silently keep an entire composite stale).
func (c *compositeSkillManager) Reload(ctx context.Context) error {
	var errs []error
	for _, src := range c.sources {
		if err := src.Reload(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
