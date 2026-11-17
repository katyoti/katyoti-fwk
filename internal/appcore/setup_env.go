// SPDX-License-Identifier: Apache-2.0

package appcore

import (
	"fmt"
	"os"
	"strings"
)

// LoadEnvFile reads an existing .env file into a map.
func LoadEnvFile(path string) map[string]string {
	result := make(map[string]string)
	data, err := os.ReadFile(path)
	if err != nil {
		return result
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if idx := strings.Index(line, "="); idx > 0 {
			key := strings.TrimSpace(line[:idx])
			val := strings.TrimSpace(line[idx+1:])
			result[key] = val
		}
	}
	return result
}

// WriteEnvFile writes a map of environment variables to a .env file.
func WriteEnvFile(path string, vars map[string]string) error {
	var lines []string
	lines = append(lines, "# Environment variables")
	lines = append(lines, "# Managed by config command. Do not commit to version control.")
	lines = append(lines, "# On Windows: use forward slashes (C:/path) or unquoted backslashes.")
	lines = append(lines, "# Do NOT use double-quoted backslashes (\"C:\\path\") — they get escaped.")
	lines = append(lines, "")

	orderedKeys := []string{
		"KT_SKILLS_HOME",
		"ANTHROPIC_API_KEY",
		"OPENAI_API_KEY",
		"ELEVEN_LABS_API_KEY",
		"SUPABASE_ANON_KEY",
	}

	written := make(map[string]bool)
	for _, key := range orderedKeys {
		if val, ok := vars[key]; ok && val != "" {
			lines = append(lines, fmt.Sprintf("%s=%s", key, val))
			written[key] = true
		}
	}
	for key, val := range vars {
		if !written[key] && val != "" {
			lines = append(lines, fmt.Sprintf("%s=%s", key, val))
		}
	}
	lines = append(lines, "")

	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o600)
}
