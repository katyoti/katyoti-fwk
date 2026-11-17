// SPDX-License-Identifier: Apache-2.0

package api

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
)

// SyncProviderService provides provider discovery and login.
type SyncProviderService interface {
	List(ctx context.Context, in ListProvidersInput) (ListProvidersOutput, error)
	Login(ctx context.Context, in LoginInput) (LoginOutput, error)
}

// SyncProviderServiceDef describes the SyncProviderService for KTSP/HTTP discovery.
// CLI commands: ktw-sync provider list, ktw-sync provider login
var SyncProviderServiceDef = meta.ServiceDef{
	Name:    "provider",
	Short:   "Storage provider discovery and login",
	Long:    "List available rclone backends and run OAuth login flows.",
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		"List":  {Short: "List available storage providers (rclone backends)", RenderTemplate: listProvidersTemplate},
		"Login": meta.Method("Run OAuth login for a storage provider (opens browser)"),
	},
}
