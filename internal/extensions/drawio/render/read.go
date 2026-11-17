// SPDX-License-Identifier: Apache-2.0

package render

import (
	drawioapi "github.com/katyoti/katyoti-fwk/internal/extensions/drawio/api"
	"github.com/katyoti/katyoti-fwk/internal/extensions/drawio/internal/archdiagram"
)

// Read parses a .drawio document (plain or compressed, first page) back
// into the model it draws: the LogicalDiagram, each thing's box, the
// pinned ids. A diagram this engine wrote reads back into the model it
// came from; a foreign one into vertices, edges and containment. A
// .drawio.png or .drawio.svg is read through the diagram it embeds.
func Read(data []byte) (*drawioapi.ReadResult, error) {
	xml, err := drawioapi.ExtractDrawioXML(data)
	if err != nil {
		return nil, err
	}
	return archdiagram.Read(xml)
}
