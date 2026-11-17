// SPDX-License-Identifier: Apache-2.0

package contrib_test

import (
	"encoding/json"

	"gopkg.in/yaml.v3"
)

func yamlToStruct(raw []byte, out any) error {
	var m map[string]any
	if err := yaml.Unmarshal(raw, &m); err != nil {
		return err
	}
	j, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return json.Unmarshal(j, out)
}
