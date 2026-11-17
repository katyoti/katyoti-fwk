// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	cwmapi "github.com/katyoti/katyoti-fwk/internal/extensions/cwm/api"
)

// Project makes the project key in repo: its project file naming the
// contributions token, its module themes and - withFolder - its
// contributions folder.
func Project(t *testing.T, repo, key, token string, withFolder bool) string {
	t.Helper()
	dir := filepath.Join(repo, key)
	dirs := []string{filepath.Join(dir, "modules", "themes")}
	if withFolder {
		dirs = append(dirs, filepath.Join(dir, "modules", "sys-contributions"))
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	body := "id: " + key + "\ntype: project\nname: " + key + "\ncapabilities:\n  contributions:\n    provider: " + token + "\n"
	if err := os.WriteFile(filepath.Join(dir, cwmapi.ProjectFileName), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// AttachProjects makes roots the attached projects the pusher reads, for t.
func AttachProjects(t *testing.T, roots ...string) {
	prev := attachedRoots
	attachedRoots = func() []string { return roots }
	t.Cleanup(func() { attachedRoots = prev })
}

// PushObject is the push of an object event of project key.
func (p *Pusher) PushObject(ctx context.Context, key, object, module string) error {
	return p.push(cwmapi.WithEventProject(ctx, key), object, module, false)
}

// TokenOf is tokenOf on the CWM service of ctx.
func (p *Pusher) TokenOf(ctx context.Context, cwm cwmapi.CWMService, key, object, module string) (string, error) {
	return p.tokenOf(ctx, cwm, key, object, module)
}
