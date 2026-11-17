// SPDX-License-Identifier: Apache-2.0

package marshal

import "gopkg.in/yaml.v3"

// yamlCodec implements Codec for YAML format.
type yamlCodec struct{}

func (yamlCodec) Unmarshal(data []byte, target any) error {
	return yaml.Unmarshal(data, target)
}

func (yamlCodec) Marshal(data any) ([]byte, error) {
	return yaml.Marshal(data)
}
