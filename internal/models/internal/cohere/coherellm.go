// SPDX-License-Identifier: Apache-2.0

package cohere

import (
	"context"
	"errors"
	"os"

	"github.com/katyoti/katyoti-fwk/internal/models/api"
	"github.com/katyoti/katyoti-fwk/internal/models/internal/cohere/internal/client"
)

var (
	ErrEmptyResponse            = errors.New("no response")
	ErrMissingToken             = errors.New("missing the COHERE_API_KEY key, set it in the COHERE_API_KEY environment variable")
	ErrUnexpectedResponseLength = errors.New("unexpected length of response")
)

type LLM struct {
	client *client.Client
}

// GenerateContent implements the Model interface.
func (o *LLM) GenerateContent(ctx context.Context, messages []api.MessageContent, options ...api.CallOption) (*api.ContentResponse, error) {
	opts := &api.CallOptions{}
	for _, opt := range options {
		opt(opts)
	}

	msg0 := messages[0]
	part := msg0.Parts[0]
	result, err := o.client.CreateGeneration(ctx, &client.GenerationRequest{
		Prompt: part.(api.TextContent).Text,
	})
	if err != nil {
		return nil, err
	}

	return &api.ContentResponse{
		Choices: []*api.ContentChoice{{Content: result.Text}},
	}, nil
}

func New(opts ...Option) (*LLM, error) {
	c, err := newClient(opts...)
	return &LLM{client: c}, err
}

func newClient(opts ...Option) (*client.Client, error) {
	options := &options{
		token:   os.Getenv(tokenEnvVarName),
		baseURL: os.Getenv(baseURLEnvVarName),
		model:   os.Getenv(modelEnvVarName),
	}

	for _, opt := range opts {
		opt(options)
	}

	if len(options.token) == 0 {
		return nil, ErrMissingToken
	}

	return client.New(options.token, options.baseURL, options.model)
}
