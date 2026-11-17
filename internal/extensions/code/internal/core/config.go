// SPDX-License-Identifier: Apache-2.0

package core

// CodeServiceConfig holds configuration for the CodeService.
type CodeServiceConfig struct {
	// DefaultExtensions is the list of file extensions to analyze by default.
	DefaultExtensions []string `yaml:"default_extensions" default:"[.go,.ts,.py,.js,.md]" desc:"File extensions to analyze"`

	// MaxFileSize is the maximum file size to read (bytes). Files larger than this are skipped.
	MaxFileSize int `yaml:"max_file_size" default:"1048576" desc:"Max file size in bytes (default 1MB)"`

	// ExcludeDirs is a list of directory patterns to exclude.
	ExcludeDirs []string `yaml:"exclude_dirs" default:"[vendor,node_modules,.git,dist,build]" desc:"Directories to exclude"`
}
