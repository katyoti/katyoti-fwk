// SPDX-License-Identifier: Apache-2.0

package indexing

import (
	"fmt"

	kbapi "github.com/katyoti/katyoti-fwk/internal/extensions/kb/api"
)

// ResolveIndexer creates a KnowledgeIndexer from the backend name and config.
func ResolveIndexer(backend string, config map[string]any) (kbapi.KnowledgeIndexer, error) {
	switch backend {
	case "chromem-go", "chromem", "":
		return NewChromemIndexer(config)
	// Future backends:
	// case "chromadb":
	//     return NewChromaDBIndexer(config)
	// case "qdrant":
	//     return NewQdrantIndexer(config)
	// case "sqlite-vec":
	//     return NewSqliteVecIndexer(config)
	default:
		return nil, fmt.Errorf("unknown indexing backend %q (supported: chromem-go)", backend)
	}
}
