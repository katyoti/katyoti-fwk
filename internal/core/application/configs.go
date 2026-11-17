// SPDX-License-Identifier: Apache-2.0

package application

// GetConfigMap returns the raw config map for a config name (init-phase, takes app).
func GetConfigMap(app *Application, configName string) (map[string]any, bool) {
	return app.cfg.GetMap(configName)
}
