// SPDX-License-Identifier: Apache-2.0

// Package mdrender handles Jinja2 template rendering and Markdown-to-DOCX conversion.
package mdrender

import (
	"fmt"

	"github.com/nikolalohinski/gonja/v2"
	"github.com/nikolalohinski/gonja/v2/exec"
)

// RenderJinja2 renders a Jinja2 template string with data using gonja.
func RenderJinja2(templateStr string, data map[string]any) (string, error) {
	tpl, err := gonja.FromString(templateStr)
	if err != nil {
		return "", fmt.Errorf("parse jinja2: %w", err)
	}
	ctx := exec.NewContext(data)
	result, err := tpl.ExecuteToString(ctx)
	if err != nil {
		return "", fmt.Errorf("execute jinja2: %w", err)
	}
	return result, nil
}
