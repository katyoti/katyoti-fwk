// SPDX-License-Identifier: Apache-2.0

package api

import contribapi "github.com/katyoti/katyoti-fwk/internal/core/contribution/api"

// ReadLayer names the ONE layer a producer's ResourceService.Read answers
// with: the source layer when the entry declares one, else the data layer.
//
// Read is the raw-source compatibility path on the wire; it opens this layer
// through the provider's Open. An aggregator that holds no bytes for a layer
// can reach the producer's Read only for THIS layer's content — or for another
// layer whose content address is the same — so the hub's routing and the
// producer's answer are one fact, stated here.
func ReadLayer(c contribapi.Contribution) string {
	if _, ok := c.SourceRef(); ok {
		return contribapi.LayerSource
	}
	return contribapi.LayerData
}
