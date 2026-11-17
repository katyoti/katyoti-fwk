// SPDX-License-Identifier: Apache-2.0

package core

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// KnowledgeBaseConfig is the parsed KnowledgeBase.yaml.
type KnowledgeBaseConfig struct {
	Name        string           `yaml:"name"`
	Description string           `yaml:"description,omitempty"`
	Sources     []string         `yaml:"sources"`
	Extraction  ExtractionConfig `yaml:"extraction"`
	Indexing    IndexingConfig   `yaml:"indexing"`
}

// ExtractionConfig controls how documents are extracted.
type ExtractionConfig struct {
	Profile       string        `yaml:"profile"`
	Concurrency   int           `yaml:"concurrency,omitempty"`
	OnMissingTool string        `yaml:"on_missing_tool,omitempty"` // fail | warn | skip
	Limits        ExtractLimits `yaml:"limits,omitempty"`
}

// ExtractLimits controls extraction boundaries.
type ExtractLimits struct {
	MaxFileSize string `yaml:"max_file_size,omitempty"` // e.g. "100MB"
	MaxPages    int    `yaml:"max_pages,omitempty"`
	MaxRows     int    `yaml:"max_rows,omitempty"`
}

// IndexingConfig controls how knowledge is indexed.
type IndexingConfig struct {
	Backend   string          `yaml:"backend"`
	Config    map[string]any  `yaml:"config,omitempty"`
	Embedding EmbeddingConfig `yaml:"embedding"`
	Chunking  ChunkingConfig  `yaml:"chunking"`
}

// EmbeddingConfig names the embedding profile of EmbeddingProfiles.yaml the
// knowledge base embeds through; an empty profile is the service default.
// Strategy and Config are read for older files and select nothing: a
// strategy (openai) embeds through the default profile.
type EmbeddingConfig struct {
	Profile  string         `yaml:"profile,omitempty"`
	Strategy string         `yaml:"strategy,omitempty"`
	Config   map[string]any `yaml:"config,omitempty"`
}

// ChunkingConfig selects the chunker strategy.
type ChunkingConfig struct {
	Strategy string         `yaml:"strategy"`
	Config   map[string]any `yaml:"config,omitempty"`
}

// SourceConfig is the parsed sources/<name>.yaml.
type SourceConfig struct {
	Adapter   string         `yaml:"adapter,omitempty"` // default: "rclone"
	Workspace map[string]any `yaml:"workspace,omitempty"`
	Job       JobConfig      `yaml:"job,omitempty"`
	Config    map[string]any `yaml:"config,omitempty"` // for sidecar adapters
}

// JobConfig holds sync job settings.
type JobConfig struct {
	Mode    string         `yaml:"mode,omitempty"`
	Filters []string       `yaml:"filters,omitempty"`
	Exclude []string       `yaml:"exclude,omitempty"`
	Options map[string]any `yaml:"options,omitempty"`
}

// ResolvedAdapter returns the adapter name, defaulting to "ktw-sync".
func (sc *SourceConfig) ResolvedAdapter() string {
	if sc.Adapter == "" {
		return "ktw-sync"
	}
	return sc.Adapter
}

// LoadKnowledgeBase reads and parses KnowledgeBase.yaml from the given KB root.
func LoadKnowledgeBase(kbRoot string) (*KnowledgeBaseConfig, error) {
	path := filepath.Join(kbRoot, "KnowledgeBase.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read KnowledgeBase.yaml: %w", err)
	}
	var cfg KnowledgeBaseConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse KnowledgeBase.yaml: %w", err)
	}
	if cfg.Name == "" {
		cfg.Name = filepath.Base(kbRoot)
	}
	if cfg.Extraction.Concurrency <= 0 {
		cfg.Extraction.Concurrency = 1
	}
	if cfg.Extraction.OnMissingTool == "" {
		cfg.Extraction.OnMissingTool = "fail"
	}
	return &cfg, nil
}

// LoadSource reads and parses a source config from sources/<name>.yaml.
func LoadSource(kbRoot, name string) (*SourceConfig, error) {
	path := filepath.Join(kbRoot, "sources", name+".yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read source config %s: %w", name, err)
	}
	var cfg SourceConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse source config %s: %w", name, err)
	}
	return &cfg, nil
}

// LoadAllSources loads all source configs referenced by the KB config.
func LoadAllSources(kbRoot string, kbCfg *KnowledgeBaseConfig) (map[string]*SourceConfig, error) {
	sources := make(map[string]*SourceConfig, len(kbCfg.Sources))
	for _, name := range kbCfg.Sources {
		src, err := LoadSource(kbRoot, name)
		if err != nil {
			return nil, err
		}
		sources[name] = src
	}
	return sources, nil
}
