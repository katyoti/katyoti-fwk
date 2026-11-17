// SPDX-License-Identifier: Apache-2.0

package api

import "github.com/katyoti/katyoti-fwk/internal/core/meta"

// EmbeddingProfilesDef describes the in-process EmbeddingProfiles service.
// It publishes no method: the service is resolved in process through
// application.UseService[EmbeddingProfiles](ctx).
var EmbeddingProfilesDef = meta.ServiceDef{
	Name:    "embedding-profiles",
	Short:   "Embedding profiles (in-process accessor only — no RPC)",
	Long:    "Named embedding endpoints read from EmbeddingProfiles.yaml: OpenAI, Microsoft Foundry, Azure OpenAI or any OpenAI-compatible server. A knowledge base names its profile; an empty name is the default profile.",
	Version: "1.0.0",
	Methods: map[string]meta.MethodDef{},
}

// EmbeddingProfiles hands out the embedder of a named embedding profile.
type EmbeddingProfiles interface {
	// Embedder returns the embedder of the named profile; an empty name is
	// the default profile. A profile whose key variable is unset is an error
	// naming the variable and the profile.
	Embedder(profile string) (Embedder, error)
	// Default is the name of the default profile.
	Default() string
	// Profiles lists the profile names, sorted.
	Profiles() []string
	// Check reports whether the named profile is ready to embed, without
	// embedding: an empty name is the default profile; an undeclared one is
	// an error. It reads whether the key variable is set, never its value,
	// and calls no endpoint.
	Check(profile string) (EmbeddingProfileCheck, error)
}

// EmbeddingProfileCheck is what Check reports of one profile.
type EmbeddingProfileCheck struct {
	Profile    string `json:"profile"`
	Type       string `json:"type"`
	Model      string `json:"model"`
	Dimensions int    `json:"dimensions"`
	KeyEnv     string `json:"key_env"`
	KeySet     bool   `json:"key_set"`
}

// EmbeddingProfilesConfig is EmbeddingProfiles.yaml: the named embedding
// endpoints and the default one.
type EmbeddingProfilesConfig struct {
	Default  string                      `yaml:"default,omitempty" desc:"Profile used when a knowledge base names none"`
	Profiles map[string]EmbeddingProfile `yaml:"profiles,omitempty" desc:"Embedding profiles by name"`
}

// EmbeddingProfile is one OpenAI-shaped embeddings endpoint.
type EmbeddingProfile struct {
	Type       string `yaml:"type" desc:"openai | foundry | azure-openai | openai-compatible"`
	Endpoint   string `yaml:"endpoint,omitempty" desc:"Full URL of the embeddings endpoint (default for openai: https://api.openai.com/v1/embeddings)"`
	KeyEnv     string `yaml:"key_env,omitempty" desc:"Name of the environment variable holding the key (default for openai: OPENAI_API_KEY)"`
	Auth       string `yaml:"auth,omitempty" desc:"bearer (default) or api-key (default for azure-openai)"`
	APIVersion string `yaml:"api_version,omitempty" desc:"Sent as ?api-version= (required for azure-openai)"`
	Model      string `yaml:"model,omitempty" desc:"Model sent in the request body (the deployment name on Foundry and Azure OpenAI)"`
	Dimensions int    `yaml:"dimensions,omitempty" desc:"Vector size the model returns (default 1536)"`
	BatchSize  int    `yaml:"batch_size,omitempty" desc:"Maximum texts per request (default 100)"`
	// APIKey exists only to refuse a key written in the file: a key is
	// never stored in configuration, its variable is named in key_env.
	APIKey string `yaml:"api_key,omitempty" desc:"Refused: name the key's environment variable in key_env"`
}
