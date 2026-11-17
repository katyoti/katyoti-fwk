// SPDX-License-Identifier: Apache-2.0

package embedding

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	kbapi "github.com/katyoti/katyoti-fwk/internal/extensions/kb/api"
)

// Profile types: every one speaks the OpenAI embeddings shape
// (POST {"input": [...], "model": ...} → {"data": [{"embedding": [...]}]}).
const (
	TypeOpenAI           = "openai"
	TypeFoundry          = "foundry"
	TypeAzureOpenAI      = "azure-openai"
	TypeOpenAICompatible = "openai-compatible"
)

// Auth schemes: how the key is sent.
const (
	AuthBearer = "bearer"  // Authorization: Bearer <key>
	AuthAPIKey = "api-key" // api-key: <key>
)

const (
	defaultOpenAIEndpoint = "https://api.openai.com/v1/embeddings"
	defaultOpenAIKeyEnv   = "OPENAI_API_KEY"
	defaultOpenAIModel    = "text-embedding-3-small"
	defaultDimensions     = 1536
	defaultProfileBatch   = 100
	defaultProfileName    = "openai"
)

// EmbeddingProfilesConfig is EmbeddingProfiles.yaml (declared in the api).
type EmbeddingProfilesConfig = kbapi.EmbeddingProfilesConfig

// Profile is one embedding endpoint.
type Profile = kbapi.EmbeddingProfile

// DefaultProfilesConfig is the configuration used when EmbeddingProfiles.yaml
// declares no profile: one OpenAI profile on OPENAI_API_KEY.
func DefaultProfilesConfig() *EmbeddingProfilesConfig {
	return &EmbeddingProfilesConfig{
		Default: defaultProfileName,
		Profiles: map[string]Profile{
			defaultProfileName: {
				Type:       TypeOpenAI,
				Endpoint:   defaultOpenAIEndpoint,
				KeyEnv:     defaultOpenAIKeyEnv,
				Model:      defaultOpenAIModel,
				Dimensions: defaultDimensions,
				BatchSize:  defaultProfileBatch,
			},
		},
	}
}

// resolvedProfile is a validated profile with its defaults applied.
type resolvedProfile struct {
	name       string
	typ        string
	url        string // endpoint with ?api-version= appended
	keyEnv     string
	auth       string
	model      string
	dimensions int
	batchSize  int
}

// resolve validates a profile and applies its defaults.
func resolve(name string, p Profile) (resolvedProfile, error) {
	fail := func(format string, args ...any) (resolvedProfile, error) {
		return resolvedProfile{}, fmt.Errorf("embedding profile %q (%s): %s", name, p.Type, fmt.Sprintf(format, args...))
	}
	if p.APIKey != "" {
		return fail("api_key is refused: a key is never written in EmbeddingProfiles.yaml, name its environment variable in key_env")
	}
	r := resolvedProfile{
		name: name, typ: p.Type, keyEnv: p.KeyEnv, auth: p.Auth, model: p.Model,
		dimensions: p.Dimensions, batchSize: p.BatchSize,
	}
	endpoint := p.Endpoint
	switch p.Type {
	case TypeOpenAI:
		if endpoint == "" {
			endpoint = defaultOpenAIEndpoint
		}
		if r.keyEnv == "" {
			r.keyEnv = defaultOpenAIKeyEnv
		}
		if r.model == "" {
			r.model = defaultOpenAIModel
		}
		if r.auth == "" {
			r.auth = AuthBearer
		}
	case TypeFoundry, TypeOpenAICompatible:
		if r.auth == "" {
			r.auth = AuthBearer
		}
	case TypeAzureOpenAI:
		if r.auth == "" {
			r.auth = AuthAPIKey
		}
		if p.APIVersion == "" {
			return fail("api_version is required")
		}
	case "":
		return fail("type is required (openai, foundry, azure-openai, openai-compatible)")
	default:
		return fail("unknown type (supported: openai, foundry, azure-openai, openai-compatible)")
	}
	if endpoint == "" {
		return fail("endpoint is required")
	}
	if r.keyEnv == "" {
		return fail("key_env is required: the name of the environment variable holding the key")
	}
	if r.model == "" {
		return fail("model is required")
	}
	if r.auth != AuthBearer && r.auth != AuthAPIKey {
		return fail("unknown auth %q (supported: bearer, api-key)", r.auth)
	}
	if r.dimensions < 0 || r.batchSize < 0 {
		return fail("dimensions and batch_size must not be negative")
	}
	if r.dimensions == 0 {
		r.dimensions = defaultDimensions
	}
	if r.batchSize == 0 {
		r.batchSize = defaultProfileBatch
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return fail("endpoint %q is not a URL", endpoint)
	}
	if p.APIVersion != "" {
		q := u.Query()
		q.Set("api-version", p.APIVersion)
		u.RawQuery = q.Encode()
	}
	r.url = u.String()
	return r, nil
}

// profiles implements kbapi.EmbeddingProfiles.
type profiles struct {
	def    string
	byName map[string]resolvedProfile
	client *http.Client
}

// NewEmbeddingProfiles validates the configuration and returns the service.
// A configuration with no profile falls back to DefaultProfilesConfig.
func NewEmbeddingProfiles(cfg *EmbeddingProfilesConfig) (kbapi.EmbeddingProfiles, error) {
	return newProfiles(cfg, &http.Client{Timeout: 120 * time.Second})
}

func newProfiles(cfg *EmbeddingProfilesConfig, client *http.Client) (*profiles, error) {
	if cfg == nil || len(cfg.Profiles) == 0 {
		fallback := DefaultProfilesConfig()
		if cfg != nil && cfg.Default != "" && cfg.Default != fallback.Default {
			return nil, fmt.Errorf("EmbeddingProfiles: default profile %q is not declared", cfg.Default)
		}
		cfg = fallback
	}
	ps := &profiles{def: cfg.Default, byName: make(map[string]resolvedProfile, len(cfg.Profiles)), client: client}
	var errs []string
	for name, p := range cfg.Profiles {
		r, err := resolve(name, p)
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		ps.byName[name] = r
	}
	if len(errs) > 0 {
		sort.Strings(errs)
		return nil, fmt.Errorf("EmbeddingProfiles: %s", strings.Join(errs, "; "))
	}
	if ps.def == "" {
		if len(ps.byName) != 1 {
			return nil, fmt.Errorf("EmbeddingProfiles: default is required when several profiles are declared (%s)", strings.Join(ps.Profiles(), ", "))
		}
		for name := range ps.byName {
			ps.def = name
		}
	}
	if _, ok := ps.byName[ps.def]; !ok {
		return nil, fmt.Errorf("EmbeddingProfiles: default profile %q is not declared (declared: %s)", ps.def, strings.Join(ps.Profiles(), ", "))
	}
	return ps, nil
}

func (ps *profiles) Default() string { return ps.def }

func (ps *profiles) Profiles() []string {
	names := make([]string, 0, len(ps.byName))
	for name := range ps.byName {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// lookup returns the named profile; an empty name is the default.
func (ps *profiles) lookup(name string) (resolvedProfile, error) {
	if name == "" {
		name = ps.def
	}
	r, ok := ps.byName[name]
	if !ok {
		return resolvedProfile{}, fmt.Errorf("embedding profile %q is not declared in EmbeddingProfiles.yaml (declared: %s)", name, strings.Join(ps.Profiles(), ", "))
	}
	return r, nil
}

func (ps *profiles) Check(name string) (kbapi.EmbeddingProfileCheck, error) {
	r, err := ps.lookup(name)
	if err != nil {
		return kbapi.EmbeddingProfileCheck{}, err
	}
	return kbapi.EmbeddingProfileCheck{
		Profile: r.name, Type: r.typ, Model: r.model, Dimensions: r.dimensions,
		KeyEnv: r.keyEnv, KeySet: os.Getenv(r.keyEnv) != "",
	}, nil
}

func (ps *profiles) Embedder(name string) (kbapi.Embedder, error) {
	r, err := ps.lookup(name)
	if err != nil {
		return nil, err
	}
	key := os.Getenv(r.keyEnv)
	if key == "" {
		return nil, fmt.Errorf("embedding profile %q (%s): the environment variable %s holding its key is not set", r.name, r.typ, r.keyEnv)
	}
	return &httpEmbedder{p: r, key: key, client: ps.client}, nil
}
