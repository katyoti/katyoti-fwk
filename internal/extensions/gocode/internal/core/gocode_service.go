// SPDX-License-Identifier: Apache-2.0

package core

import (
	stdctx "context"

	"github.com/katyoti/katyoti-fwk/internal/extensions/gocode/api"
)

// GocodeServiceImpl implements api.GocodeService. Each verb lives in
// its methods_<group>.go file (module create/discover/status/verify/
// audit/lint/promote, use cases, features, tasktests).
type GocodeServiceImpl struct {
	cfg *Config
}

// NewGocodeService is the factory consumed by appcore.PublishService.
// Signature matches publish.Factory[api.GocodeService, Config].
func NewGocodeService(_ stdctx.Context, cfg *Config) (api.GocodeService, error) {
	if cfg == nil {
		cfg = &Config{}
	}
	return &GocodeServiceImpl{cfg: cfg}, nil
}
