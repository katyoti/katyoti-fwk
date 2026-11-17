// SPDX-License-Identifier: Apache-2.0

//go:build windows

package shortcut

import (
	"fmt"

	"golang.org/x/sys/windows/registry"
)

// RegisterScheme makes scheme open target for the person: under
// HKCU\Software\Classes\<scheme> - « URL Protocol », and shell\open\command
// running `"<target>" gui --open "%1"`. The last installation set up owns the
// scheme. Answers what it wrote.
func RegisterScheme(scheme, target string) (string, error) {
	path := `Software\Classes\` + scheme
	k, _, err := registry.CreateKey(registry.CURRENT_USER, path, registry.SET_VALUE)
	if err != nil {
		return "", fmt.Errorf("register %s://: %w", scheme, err)
	}
	defer k.Close()
	if err := k.SetStringValue("", "URL:KaTyoti"); err != nil {
		return "", err
	}
	if err := k.SetStringValue("URL Protocol", ""); err != nil {
		return "", err
	}
	cmd, _, err := registry.CreateKey(registry.CURRENT_USER, path+`\shell\open\command`, registry.SET_VALUE)
	if err != nil {
		return "", fmt.Errorf("register %s://: %w", scheme, err)
	}
	defer cmd.Close()
	if err := cmd.SetStringValue("", linkCommand(target, "%1")); err != nil {
		return "", err
	}
	return `HKCU\` + path, nil
}
