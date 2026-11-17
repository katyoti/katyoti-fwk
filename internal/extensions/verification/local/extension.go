// SPDX-License-Identifier: Apache-2.0

// Package local wires the verification service into a binary
// (`<provider>:verification/...`). Its CWM pack — the `verification`
// capability, which adds `verify()` and `lint()` to expressions — is
// contrib.Pack. It needs a CWMService in the binary: the points are CWM's.
package local

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/publish"
	"github.com/katyoti/katyoti-fwk/internal/extensions/verification/api"
	verificationcore "github.com/katyoti/katyoti-fwk/internal/extensions/verification/internal/core"
)

// Extension publishes the VerificationService over KTSP.
func Extension() application.Extension {
	return publish.PublishSimpleService[api.VerificationService](
		func(context.Context) (api.VerificationService, error) { return verificationcore.New(), nil },
		publish.WithServiceDef(api.VerificationServiceDef),
		publish.WithKTSPContainer(),
	)
}
