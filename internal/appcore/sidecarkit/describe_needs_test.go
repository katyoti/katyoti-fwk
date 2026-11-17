// SPDX-License-Identifier: Apache-2.0

package sidecarkit_test

import (
	"context"
	"testing"
	"testing/fstest"

	"github.com/katyoti/katyoti-fwk/internal/appcore"
	"github.com/katyoti/katyoti-fwk/internal/appcore/sidecarkit"
	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/publish"
)

// The keys and logins a sidecar's services declare are carried by
// `describe --out json`, beside its programs: what a catalog copies into the
// item's requires.
func TestDescribe_CarriesKeysAndLogins(t *testing.T) {
	t.Setenv("KTW_WORKSPACE", t.TempDir())
	t.Setenv("KTW_DATA", t.TempDir())
	t.Setenv("KTW_CACHE", t.TempDir())
	app := application.New(application.Config(), sidecarkit.NewSidecar(
		appcore.AppConfig{AppName: "ktw-fixture", Version: "1.0.0", EmbeddedFS: fstest.MapFS{}},
		sidecarkit.WithDomain(publish.PublishSimpleService[recipeFixtureSvc](
			func(context.Context) (recipeFixtureSvc, error) { return recipeFixtureImpl{}, nil },
			publish.WithTools(fixtureOptional),
			publish.WithAPIKeys(
				publish.APIKeyDef{EnvName: "FIXTURE_KEY", Label: "Fixture key", Required: true},
				publish.APIKeyDef{EnvName: "FIXTURE_EXTRA"},
			),
			publish.WithAuth(publish.AuthDef{Name: "fixture-login", Label: "Fixture login", LoginHint: "ktw-fixture login"}),
		)),
		sidecarkit.WithoutPack(), sidecarkit.WithoutKTSP(), sidecarkit.WithoutHTTPServer(),
	)...)
	if err := app.Run(); err != nil {
		t.Fatalf("app.Run: %v", err)
	}
	t.Cleanup(func() { appcore.CloseApplicationScope(application.WithApplication(context.Background(), app)) })

	d := describeJSON(t, app.Context())
	if len(d.APIKeys) != 2 || d.APIKeys[0].EnvName != "FIXTURE_KEY" || !d.APIKeys[0].Required ||
		d.APIKeys[0].Label != "Fixture key" || d.APIKeys[1].EnvName != "FIXTURE_EXTRA" || d.APIKeys[1].Required {
		t.Errorf("describe keys = %+v", d.APIKeys)
	}
	if len(d.Auth) != 1 || d.Auth[0].Name != "fixture-login" || d.Auth[0].LoginHint != "ktw-fixture login" {
		t.Errorf("describe logins = %+v", d.Auth)
	}
	if len(d.Tools) != 1 || d.Tools[0].Name != "fixture-media" {
		t.Errorf("describe programs = %+v", d.Tools)
	}
}
