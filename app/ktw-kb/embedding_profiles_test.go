// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"testing"

	"github.com/katyoti/katyoti-fwk/app/ktw-kb/cmd"
	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/config"
	"github.com/katyoti/katyoti-fwk/internal/core/marshal"
	kbapi "github.com/katyoti/katyoti-fwk/internal/extensions/kb/api"
	"github.com/katyoti/katyoti-fwk/internal/test/census"
)

// The shipped EmbeddingProfiles.yaml decodes through the framework decoder
// and its default is the openai profile on OPENAI_API_KEY; the binary
// resolves the EmbeddingProfiles service through application, its default
// openai.
func TestShippedEmbeddingProfilesDefaultIsOpenAI(t *testing.T) {
	data, err := embeddedFS.ReadFile("embed/config/EmbeddingProfiles.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var cfg kbapi.EmbeddingProfilesConfig
	if err := config.DecodeBytes(data, marshal.YAML, &cfg); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if cfg.Default != "openai" {
		t.Fatalf("default %q, want openai", cfg.Default)
	}
	if len(cfg.Profiles) != 1 {
		t.Errorf("%d profiles shipped, want the openai one only (the others are commented examples)", len(cfg.Profiles))
	}
	p, ok := cfg.Profiles["openai"]
	if !ok || p.Type != "openai" || p.KeyEnv != "OPENAI_API_KEY" || p.Model != "text-embedding-3-small" ||
		p.Dimensions != 1536 || p.BatchSize != 100 || p.Endpoint != "https://api.openai.com/v1/embeddings" {
		t.Fatalf("openai profile %+v", p)
	}

	census.Isolate(t)
	app := application.New(application.Config(), cmd.Extensions(embeddedFS)...)
	if err := app.Run(); err != nil {
		t.Fatalf("app.Run: %v", err)
	}
	t.Cleanup(func() { appcore.CloseApplicationScope(application.WithApplication(context.Background(), app)) })
	svc, err := application.UseService[kbapi.EmbeddingProfiles](app.Context())
	if err != nil {
		t.Fatalf("resolve EmbeddingProfiles: %v", err)
	}
	if svc.Default() != "openai" {
		t.Fatalf("service default %q, want openai", svc.Default())
	}
}
