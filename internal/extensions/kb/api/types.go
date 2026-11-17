// SPDX-License-Identifier: Apache-2.0

// Package api defines the public interfaces for the ktw-kb knowledge pipeline.
package api

import (
	"context"

	"github.com/katyoti/katyoti-fwk/internal/core/meta"
)

// KBServiceDef describes the published KBService for KTSP/HTTP discovery.
var KBServiceDef = meta.ServiceDef{
	Name:  "kb",
	Short: "Knowledge base pipeline",
	Long: `Manages knowledge bases: sync document sources, extract knowledge bundles,
chunk and embed content, index into vector databases, and query with full
source traceability.`,
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{
		"Create":         meta.Method("Create a new knowledge base with template config"),
		"Sync":           {Short: "Sync all sources (calls ktw-sync for each source)", TimeoutS: 300},
		"Extract":        {Short: "Extract knowledge bundles from synced documents", TimeoutS: 300},
		"Index":          {Short: "Index extracted bundles into vector DB", TimeoutS: 300},
		"Run":            {Short: "Run full pipeline: sync → extract → index", TimeoutS: 600},
		"Login":          meta.Method("Authorize a remote source (OAuth login)"),
		"Status":         {Short: "Show KB status: sources, bundles, index, tools", RenderTemplate: statusTmpl},
		"Query":          {Short: "Search the knowledge base", RenderTemplate: queryTmpl, TimeoutS: 60},
		"EmbeddingCheck": meta.Method("Check an embedding profile is ready without embedding: its type, model, dimensions and key variable; fails naming the variable when it is not set"),
	},
}

// KBService is the top-level service for knowledge base operations.
type KBService interface {
	Create(ctx context.Context, in CreateInput) (CreateOutput, error)
	Sync(ctx context.Context, in SyncInput) (SyncOutput, error)
	Extract(ctx context.Context, in ExtractInput) (ExtractOutput, error)
	Index(ctx context.Context, in IndexInput) (IndexOutput, error)
	Run(ctx context.Context, in RunInput) (RunOutput, error)
	Login(ctx context.Context, in LoginInput) (LoginOutput, error)
	Status(ctx context.Context, in StatusInput) (StatusOutput, error)
	Query(ctx context.Context, in QueryInput) (QueryOutput, error)
	EmbeddingCheck(ctx context.Context, in EmbeddingCheckInput) (EmbeddingProfileCheck, error)
}

// --- EmbeddingCheck ---

type EmbeddingCheckInput struct {
	Profile string `json:"profile,omitempty"` // empty = the default profile
}

// --- Create ---

type CreateInput struct {
	Path string `json:"path"` // directory to create KB in
}

type CreateOutput struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

// --- Sync ---

type SyncInput struct {
	Path    string   `json:"path"`
	Sources []string `json:"sources,omitempty"` // empty = all sources
}

type SyncOutput struct {
	SourcesSynced int      `json:"sources_synced"`
	Errors        []string `json:"errors,omitempty"`
}

// --- Extract ---

type ExtractInput struct {
	Path string `json:"path"`
}

type ExtractOutput struct {
	New     int      `json:"new"`
	Updated int      `json:"updated"`
	Skipped int      `json:"skipped"`
	Failed  int      `json:"failed"`
	Errors  []string `json:"errors,omitempty"`
}

// --- Index ---

type IndexInput struct {
	Path  string `json:"path"`
	Force bool   `json:"force,omitempty"` // force re-index (drop collection)
}

type IndexOutput struct {
	ChunksIndexed int      `json:"chunks_indexed"`
	ChunksDeleted int      `json:"chunks_deleted"`
	Errors        []string `json:"errors,omitempty"`
}

// --- Run (full pipeline) ---

type RunInput struct {
	Path string `json:"path"`
}

type RunOutput struct {
	Sync    SyncOutput    `json:"sync"`
	Extract ExtractOutput `json:"extract"`
	Index   IndexOutput   `json:"index"`
}

// --- Status ---

type StatusInput struct {
	Path string `json:"path"`
}

type StatusOutput struct {
	Name           string          `json:"name"`
	Sources        []SourceStatus  `json:"sources"`
	Bundles        BundleCounts    `json:"bundles"`
	Index          IndexStatus     `json:"index"`
	ToolsAvailable map[string]bool `json:"tools_available"`
}

type SourceStatus struct {
	Name    string `json:"name"`
	Adapter string `json:"adapter"`
	Files   int    `json:"files"`
}

type BundleCounts struct {
	Extracted int `json:"extracted"`
	Failed    int `json:"failed"`
	Stale     int `json:"stale"`
	Pending   int `json:"pending"`
}

type IndexStatus struct {
	Backend        string `json:"backend"`
	Collection     string `json:"collection"`
	EmbeddingModel string `json:"embedding_model"`
	ChunksStored   int    `json:"chunks_stored"`
}

// --- Login ---

type LoginInput struct {
	Path   string `json:"path"`   // KB root
	Source string `json:"source"` // source name to authorize
}

type LoginOutput struct {
	Source  string `json:"source"`
	Message string `json:"message"`
}

// --- Query ---

type QueryInput struct {
	Path   string            `json:"path"`
	Query  string            `json:"query"`
	Limit  int               `json:"limit,omitempty"` // default 5
	Filter map[string]string `json:"filter,omitempty"`
}

type QueryOutput struct {
	Results []QueryResult `json:"results"`
}

type QueryResult struct {
	Content string     `json:"content"`
	Score   float64    `json:"score"`
	Source  SourceInfo `json:"source"`
}

type SourceInfo struct {
	KBName      string `json:"kb_name"`
	SourcePath  string `json:"source_path"`
	RemotePath  string `json:"remote_path,omitempty"`
	SyncSource  string `json:"sync_source,omitempty"`
	Part        string `json:"part"`
	PartIndex   int    `json:"part_index"`
	ChunkIndex  int    `json:"chunk_index"`
	ExtractedAt string `json:"extracted_at"`
}
