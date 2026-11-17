// SPDX-License-Identifier: Apache-2.0

package methodsservice

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
)

// Extension publishes MethodsService (KTSP + flat CLI). Wire it into the binary
// that hosts the methods-pack services — ktw-methods — so the linters +
// verifiers are dispatchable as `ktw-methods:methods/<verb>` from any node/item
// type's `linter:` / `verifier:` declarations.
func Extension() application.Extension {
	factory := func(_ context.Context, _ *Config) (MethodsService, error) {
		return New(), nil
	}
	return appcore.PublishService[MethodsService, Config](
		factory,
		appcore.WithServiceDef(MethodsServiceDef),
		appcore.WithKTSPContainer(),
	)
}
