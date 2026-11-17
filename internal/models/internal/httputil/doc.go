// SPDX-License-Identifier: Apache-2.0

// Package httputil provides HTTP transport and client utilities for LLM providers.
//
// # Default HTTP Client
//
// The package provides DefaultClient, a pre-configured http.Client
// that includes a User-Agent header identifying the application:
//
//	resp, err := httputil.DefaultClient.Get("https://api.example.com/data")
//
// # Logging and Debugging
//
// For development and debugging, the package provides logging clients:
//
//	client := httputil.LoggingClient    // logs full HTTP requests/responses via slog
//	client := httputil.JSONDebugClient  // pretty-prints JSON + SSE with ANSI colors
//
// # Image Download
//
// DownloadImageData fetches an image URL and returns its MIME subtype + data.
package httputil
