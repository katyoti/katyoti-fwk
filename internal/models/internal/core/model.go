// SPDX-License-Identifier: Apache-2.0

// Package core re-exports api types as aliases for backward compatibility.
// All types are defined in models/api.
package core

import "github.com/katyoti/katyoti-fwk/internal/models/api"

type Model = api.Model
type ModelInfo = api.ModelInfo
type ModelType = api.ModelType
type LLMProviders = api.LLMProviders
