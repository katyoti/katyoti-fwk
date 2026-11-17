// SPDX-License-Identifier: Apache-2.0

package core

// TaskServiceConfig is loaded from TaskService.yaml.
type TaskServiceConfig struct {
	ExtraDirs []string `yaml:"extra_dirs"`
}
