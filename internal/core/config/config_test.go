// SPDX-License-Identifier: Apache-2.0

package config

import (
	"testing"

	"github.com/katyoti/katyoti-fwk/internal/core/loader"
)

func TestNewConfigService(t *testing.T) {
	svc := NewConfigService(loader.NewMapLoader(nil))

	if svc == nil {
		t.Fatal("expected non-nil ConfigService")
	}

	// Verify basic config operations work.
	val := svc.Get("nonexistent.key", "default")
	if val != "default" {
		t.Errorf("Get returned %v, want default", val)
	}
}
