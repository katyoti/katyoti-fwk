// SPDX-License-Identifier: Apache-2.0

package archdiagram

import "html"

// xmlAttr encodes a string for use as an XML attribute value.
// HTML tags and special characters are entity-encoded.
func xmlAttr(s string) string {
	return html.EscapeString(s)
}

const drawioFooter = `      </root>
    </mxGraphModel>
  </diagram>
</mxfile>
`
