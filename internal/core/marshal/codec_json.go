// SPDX-License-Identifier: Apache-2.0

package marshal

import "encoding/json"

// jsonCodec implements Codec for JSON format.
type jsonCodec struct{}

func (jsonCodec) Unmarshal(data []byte, target any) error {
	return json.Unmarshal(data, target)
}

func (jsonCodec) Marshal(data any) ([]byte, error) {
	return json.MarshalIndent(data, "", "  ")
}
