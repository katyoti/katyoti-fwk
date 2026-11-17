// SPDX-License-Identifier: Apache-2.0

package core

import (
	"fmt"
	"sort"
	"strings"

	"github.com/rclone/rclone/fs"

	syncapi "github.com/katyoti/katyoti-fwk/internal/extensions/sync/api"

	// Import selected rclone backends — covers 90%+ of use cases.
	// Adding backend/all registers ~60 backends and adds ~10s to startup.
	_ "github.com/rclone/rclone/backend/drive"    // Google Drive
	_ "github.com/rclone/rclone/backend/ftp"      // FTP/FTPS
	_ "github.com/rclone/rclone/backend/local"    // Local filesystem
	_ "github.com/rclone/rclone/backend/onedrive" // OneDrive / SharePoint
	_ "github.com/rclone/rclone/backend/s3"       // AWS S3 + S3-compatible (MinIO, Backblaze, Wasabi, etc.)
	_ "github.com/rclone/rclone/backend/sftp"     // SSH/SFTP
	_ "github.com/rclone/rclone/backend/webdav"   // WebDAV (Nextcloud, ownCloud, Box, NAS)
)

// ListProviders returns available rclone backends, optionally filtered by fuzzy query.
func ListProviders(query string) syncapi.ListProvidersOutput {
	query = strings.ToLower(strings.TrimSpace(query))

	var providers []syncapi.ProviderInfo
	for _, ri := range fs.Registry {
		if ri.Hide {
			continue
		}
		if query != "" {
			if !fuzzyMatch(query, strings.ToLower(ri.Name)) &&
				!fuzzyMatch(query, strings.ToLower(ri.Description)) {
				continue
			}
		}
		desc := ri.Description
		if len(desc) > 40 {
			desc = desc[:37] + "..."
		}
		providers = append(providers, syncapi.ProviderInfo{
			Name:        ri.Name,
			Description: desc,
		})
	}
	sort.Slice(providers, func(i, j int) bool {
		return providers[i].Name < providers[j].Name
	})
	return syncapi.ListProvidersOutput{Providers: providers}
}

// fuzzyMatch checks if all characters of query appear in target in order.
func fuzzyMatch(query, target string) bool {
	qi := 0
	for ti := 0; ti < len(target) && qi < len(query); ti++ {
		if target[ti] == query[qi] {
			qi++
		}
	}
	return qi == len(query)
}

// isValidProviderType checks if the type name matches a registered rclone backend.
func isValidProviderType(typeName string) bool {
	for _, ri := range fs.Registry {
		if ri.Name == typeName {
			return true
		}
	}
	return false
}

// findProvider returns the RegInfo for a provider type, or nil.
func findProvider(typeName string) *fs.RegInfo {
	for _, ri := range fs.Registry {
		if ri.Name == typeName {
			return ri
		}
	}
	return nil
}

// generateWorkspaceTemplate creates a commented YAML template from rclone's option definitions.
// Required options are uncommented with empty values. Optional options are commented out.
func generateWorkspaceTemplate(name, typeName string) string {
	ri := findProvider(typeName)
	if ri == nil {
		return fmt.Sprintf("# Workspace: %s\ntype: %s\n", name, typeName)
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("# Workspace: %s\n", name))
	b.WriteString(fmt.Sprintf("# Provider: %s — %s\n", ri.Name, ri.Description))
	b.WriteString("#\n")
	b.WriteString("# Required fields are uncommented. Optional fields are commented.\n")
	b.WriteString("# Edit this file, then use 'authorize' if OAuth is needed.\n\n")
	b.WriteString(fmt.Sprintf("type: %s\n", typeName))

	// Separate required and optional options
	var required, optional []fs.Option
	for _, opt := range ri.Options {
		if opt.Hide&fs.OptionHideConfigurator != 0 {
			continue // skip hidden options
		}
		if opt.Required {
			required = append(required, opt)
		} else {
			optional = append(optional, opt)
		}
	}

	if len(required) > 0 {
		b.WriteString("\n# --- Required ---\n")
		for _, opt := range required {
			writeOption(&b, opt, false)
		}
	}

	if len(optional) > 0 {
		b.WriteString("\n# --- Optional ---\n")
		for _, opt := range optional {
			writeOption(&b, opt, true)
		}
	}

	return b.String()
}

func writeOption(b *strings.Builder, opt fs.Option, commented bool) {
	// Description as comment
	if opt.Help != "" {
		help := strings.TrimSpace(opt.Help)
		for _, line := range strings.Split(help, "\n") {
			b.WriteString("# " + line + "\n")
		}
	}

	// Value line
	prefix := ""
	if commented {
		prefix = "# "
	}

	defaultVal := ""
	if opt.Default != nil {
		defaultVal = fmt.Sprintf("%v", opt.Default)
	}

	name := opt.Name
	if defaultVal != "" && defaultVal != "false" && defaultVal != "0" {
		b.WriteString(fmt.Sprintf("%s%s: %s\n", prefix, name, defaultVal))
	} else {
		b.WriteString(fmt.Sprintf("%s%s: \n", prefix, name))
	}
	b.WriteString("\n")
}
