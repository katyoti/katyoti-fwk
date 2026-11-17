// SPDX-License-Identifier: Apache-2.0

package core

import (
	"context"
	"fmt"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	"github.com/katyoti/katyoti-fwk/internal/core/loader"
	syncapi "github.com/katyoti/katyoti-fwk/internal/extensions/sync/api"
)

// WorkspaceConfig is the on-disk representation of a workspace.
type WorkspaceConfig struct {
	Type string         `yaml:"type"`
	Rest map[string]any `yaml:",inline"` // provider-specific fields
}

// WorkspaceLibrary manages workspace YAML files via Library[T].
type WorkspaceLibrary struct {
	lib *application.Library[WorkspaceConfig]
}

// WorkspaceLibraryExtension registers WorkspaceLibrary as a DI service.
var WorkspaceLibraryExtension = application.DeclareSimpleService[*WorkspaceLibrary](
	func(ctx context.Context) (*WorkspaceLibrary, error) {
		env, err := application.UseService[application.AppEnvironment](ctx)
		if err != nil {
			return nil, fmt.Errorf("WorkspaceLibrary: resolve AppEnvironment: %w", err)
		}
		dir := env.GetHomeSubDir("sync")
		ldr := loader.NewFSLoader(dir)
		return &WorkspaceLibrary{
			lib: application.NewLibrary[WorkspaceConfig]("workspaces", ldr),
		}, nil
	},
)

func (wl *WorkspaceLibrary) Create(in syncapi.CreateWorkspaceInput) (syncapi.CreateWorkspaceOutput, error) {
	if in.Name == "" {
		return syncapi.CreateWorkspaceOutput{}, fmt.Errorf("workspace name is required (--name)")
	}
	if in.Type == "" {
		return syncapi.CreateWorkspaceOutput{}, fmt.Errorf("provider type is required (--type). Use 'list-providers' to see available types")
	}

	// Validate provider type against rclone registry
	if !isValidProviderType(in.Type) {
		return syncapi.CreateWorkspaceOutput{}, fmt.Errorf("unknown provider type %q. Use 'list-providers' to see available types", in.Type)
	}

	if wl.lib.Exists(in.Name) {
		return syncapi.CreateWorkspaceOutput{}, fmt.Errorf("workspace %q already exists", in.Name)
	}

	// Generate a commented YAML template from rclone's option definitions
	yaml := generateWorkspaceTemplate(in.Name, in.Type)

	// Write directly (not via Library.Save — we need raw YAML with comments)
	wldr, err := wl.lib.Writable()
	if err != nil {
		return syncapi.CreateWorkspaceOutput{}, err
	}
	if err := wldr.WriteFile("workspaces/"+in.Name+".yaml", []byte(yaml)); err != nil {
		return syncapi.CreateWorkspaceOutput{}, err
	}

	return syncapi.CreateWorkspaceOutput{Name: in.Name}, nil
}

func (wl *WorkspaceLibrary) List() (syncapi.ListWorkspacesOutput, error) {
	var workspaces []syncapi.WorkspaceInfo
	for _, name := range wl.lib.List() {
		m, err := wl.lib.LoadMap(name)
		if err != nil {
			continue
		}
		wsType, _ := m["type"].(string)
		workspaces = append(workspaces, syncapi.WorkspaceInfo{
			Name: name, Type: wsType, Path: resolveWorkspacePath(m),
		})
	}
	return syncapi.ListWorkspacesOutput{Workspaces: workspaces}, nil
}

func (wl *WorkspaceLibrary) Get(name string) (syncapi.GetWorkspaceOutput, error) {
	m, err := wl.lib.LoadMap(name)
	if err != nil {
		return syncapi.GetWorkspaceOutput{}, fmt.Errorf("workspace %q: %w", name, err)
	}
	if m == nil {
		return syncapi.GetWorkspaceOutput{}, fmt.Errorf("workspace %q not found", name)
	}
	wsType, _ := m["type"].(string)
	return syncapi.GetWorkspaceOutput{Name: name, Type: wsType, Config: m}, nil
}

func (wl *WorkspaceLibrary) Delete(name string) (syncapi.DeleteWorkspaceOutput, error) {
	if !wl.lib.Exists(name) {
		return syncapi.DeleteWorkspaceOutput{Name: name}, fmt.Errorf("workspace %q not found", name)
	}
	if err := wl.lib.Delete(name); err != nil {
		return syncapi.DeleteWorkspaceOutput{Name: name}, err
	}
	return syncapi.DeleteWorkspaceOutput{Name: name, Deleted: true}, nil
}

func (wl *WorkspaceLibrary) LoadConfig(name string) (map[string]any, error) {
	return wl.lib.LoadMap(name)
}

func resolveWorkspacePath(cfg map[string]any) string {
	for _, key := range []string{"path", "remote_path", "folder_id", "bucket", "host"} {
		if p, ok := cfg[key].(string); ok && p != "" {
			return p
		}
	}
	return ""
}

func addProviderDefaults(providerType string, cfg map[string]any) {
	switch providerType {
	case "local":
		cfg["path"] = "/path/to/local/folder"
	case "drive":
		cfg["scope"] = "drive"
		cfg["folder_id"] = ""
	case "s3":
		cfg["provider"] = "AWS"
		cfg["region"] = "eu-west-1"
		cfg["bucket"] = ""
	case "sftp":
		cfg["host"] = "hostname"
		cfg["user"] = ""
		cfg["port"] = 22
	case "onedrive":
		cfg["drive_type"] = "personal"
	case "webdav":
		cfg["url"] = "https://example.com/remote.php/webdav/"
		cfg["vendor"] = "nextcloud"
	case "ftp":
		cfg["host"] = "hostname"
		cfg["port"] = 21
	case "minio":
		cfg["provider"] = "Minio"
		cfg["endpoint"] = "http://localhost:9000"
		cfg["bucket"] = ""
	}
}
