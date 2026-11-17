// SPDX-License-Identifier: Apache-2.0

// Package rest_api implements a generic REST API fetch tool.
//
// The tool is config-driven: each endpoint is defined in RestAPIs.yaml
// with URL, headers, method, query params, etc. The LLM calls the tool
// with an endpoint name and optional overrides, and the result is saved
// to a file in the workspace.
//
// Example config (RestAPIs.yaml):
//
//	endpoints:
//	  ai-models:
//	    url: https://artificialanalysis.ai/api/v2/data/llms/models
//	    method: GET
//	    headers:
//	      x-api-key: "${{env:ARTIFICIAL_ANALYSIS_AI_API_KEY}}"
//	    save_as: ai-models.json
//	    description: "Fetch LLM model benchmarks and pricing from Artificial Analysis"
package rest_api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/katyoti/katyoti-fwk/internal/core/application"
	toolsapi "github.com/katyoti/katyoti-fwk/internal/extensions/tools/api"
)

// ── Config ──────────────────────────────────────────────────────────────

// RestAPIsConfig is loaded from RestAPIs.yaml.
type RestAPIsConfig struct {
	Endpoints map[string]EndpointConfig `yaml:"endpoints" json:"endpoints"`
}

// EndpointConfig defines a single REST API endpoint.
type EndpointConfig struct {
	URL         string            `yaml:"url"         json:"url"`
	Method      string            `yaml:"method"      json:"method"`        // GET, POST. Default: GET
	Headers     map[string]string `yaml:"headers"     json:"headers"`       // e.g. x-api-key: "${ENV_VAR}"
	QueryParams map[string]string `yaml:"query_params" json:"query_params"` // default query parameters
	Body        string            `yaml:"body"        json:"body"`          // request body for POST
	SaveAs      string            `yaml:"save_as"     json:"save_as"`       // default filename to save as
	Description string            `yaml:"description" json:"description"`   // shown to LLM
	TimeoutSec  int               `yaml:"timeout_sec" json:"timeout_sec"`   // default: 30
}

// ── Tool ────────────────────────────────────────────────────────────────

// FetchInput is the tool's input schema.
type FetchInput struct {
	// Endpoint is the name of the configured endpoint (from RestAPIs.yaml).
	Endpoint string `json:"endpoint" jsonschema:"Name of the configured API endpoint" required:"true"`

	// QueryParams are additional query parameters to merge with the configured defaults.
	QueryParams map[string]string `json:"query_params,omitempty" jsonschema:"Additional query parameters to add to the request"`

	// SaveAs overrides the default save filename.
	SaveAs string `json:"save_as,omitempty" jsonschema:"Filename to save the response (default from config)"`
}

// FetchOutput is the tool's output.
type FetchOutput struct {
	FilePath   string `json:"file_path"         jsonschema:"Path where the response was saved"`
	StatusCode int    `json:"status_code"       jsonschema:"HTTP status code"`
	Size       int64  `json:"size"              jsonschema:"Response size in bytes"`
	Preview    string `json:"preview,omitempty" jsonschema:"First 500 chars of the response"`
	Endpoint   string `json:"endpoint"          jsonschema:"Endpoint name that was called"`
}

// NewRestAPITool creates the rest-api-fetch tool.
func NewRestAPITool() toolsapi.Tool {
	return toolsapi.NewFunctionTool(
		"rest-api-fetch",
		toolDescription,
		executeFetch,
		toolsapi.WithMutating(),
		toolsapi.WithCancelOnInterrupt(),
		toolsapi.WithDescriber(describeFetch),
		toolsapi.WithDisplay(toolsapi.ToolDisplay{Icon: "🔗", Label: "API", Color: "#82AAFF", Template: "{{.method}} {{.url}}"}),
	)
}

func executeFetch(ctx context.Context, in FetchInput) (FetchOutput, error) {
	if in.Endpoint == "" {
		return FetchOutput{}, fmt.Errorf("rest-api-fetch: endpoint name is required")
	}

	// Load config.
	cfg, err := application.UseConfig[RestAPIsConfig](ctx)
	if err != nil {
		return FetchOutput{}, fmt.Errorf("rest-api-fetch: load config: %w", err)
	}

	ep, ok := cfg.Endpoints[in.Endpoint]
	if !ok {
		available := make([]string, 0, len(cfg.Endpoints))
		for k := range cfg.Endpoints {
			available = append(available, k)
		}
		return FetchOutput{}, fmt.Errorf("rest-api-fetch: unknown endpoint %q (available: %s)", in.Endpoint, strings.Join(available, ", "))
	}

	if ep.URL == "" {
		return FetchOutput{}, fmt.Errorf("rest-api-fetch: endpoint %q has no URL configured", in.Endpoint)
	}

	// Build URL with query params.
	u, err := url.Parse(ep.URL)
	if err != nil {
		return FetchOutput{}, fmt.Errorf("rest-api-fetch: invalid URL %q: %w", ep.URL, err)
	}
	q := u.Query()
	for k, v := range ep.QueryParams {
		q.Set(k, v)
	}
	for k, v := range in.QueryParams {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()

	// Build request.
	method := ep.Method
	if method == "" {
		method = "GET"
	}

	var bodyReader io.Reader
	if ep.Body != "" && method == "POST" {
		bodyReader = strings.NewReader(ep.Body)
	}

	timeout := time.Duration(ep.TimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, method, u.String(), bodyReader)
	if err != nil {
		return FetchOutput{}, fmt.Errorf("rest-api-fetch: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	for k, v := range ep.Headers {
		req.Header.Set(k, v)
	}

	// Execute.
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return FetchOutput{}, fmt.Errorf("rest-api-fetch: request to %s failed: %w", in.Endpoint, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return FetchOutput{}, fmt.Errorf("rest-api-fetch: read response: %w", err)
	}

	// Pretty-print JSON if possible.
	var prettyData []byte
	var raw json.RawMessage
	if json.Unmarshal(data, &raw) == nil {
		prettyData, _ = json.MarshalIndent(raw, "", "  ")
	}
	if len(prettyData) == 0 {
		prettyData = data
	}

	// Determine save path.
	saveName := in.SaveAs
	if saveName == "" {
		saveName = ep.SaveAs
	}
	if saveName == "" {
		saveName = in.Endpoint + ".json"
	}

	// Save to workspace.
	dir := filepath.Dir(saveName)
	if dir != "." && dir != "" {
		os.MkdirAll(dir, 0o755)
	}
	if err := os.WriteFile(saveName, prettyData, 0o644); err != nil {
		return FetchOutput{}, fmt.Errorf("rest-api-fetch: write file: %w", err)
	}

	// Preview.
	preview := string(prettyData)
	if len(preview) > 500 {
		preview = preview[:500] + "..."
	}

	return FetchOutput{
		FilePath:   saveName,
		StatusCode: resp.StatusCode,
		Size:       int64(len(prettyData)),
		Preview:    preview,
		Endpoint:   in.Endpoint,
	}, nil
}

func describeFetch(raw json.RawMessage) string {
	var in FetchInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return "rest-api-fetch <invalid>"
	}
	return fmt.Sprintf("rest-api-fetch endpoint=%s", in.Endpoint)
}

const toolDescription = `Fetches data from a configured REST API endpoint and saves the response to a file.

Endpoints are pre-configured in RestAPIs.yaml with URL, headers, authentication, and default parameters. The LLM selects an endpoint by name and optionally adds query parameters.

# Instructions
- Use 'endpoint' to select which API to call (configured by the operator).
- Optionally add 'query_params' to filter or customize the request.
- Optionally override 'save_as' to change the output filename.
- The response is saved as a JSON file in the workspace.
- The tool returns a preview of the first 500 characters plus the file path.

# Available endpoints
Check the 'endpoint' parameter description — available endpoints depend on the system configuration.
`
