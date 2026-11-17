// SPDX-License-Identifier: Apache-2.0

//go:build windows

package shortcut

import (
	"fmt"
	"os"
	"testing"

	"golang.org/x/sys/windows/registry"
)

// The link scheme is registered for the person: a URL protocol
// whose open command runs the installation's katyoti `gui --open` with the
// link. A scheme of the test's own, removed after.
func TestTheLinkSchemeOpensTheGUI(t *testing.T) {
	scheme := fmt.Sprintf("katyoti-test-%d", os.Getpid())
	path := `Software\Classes\` + scheme
	t.Cleanup(func() {
		_ = registry.DeleteKey(registry.CURRENT_USER, path+`\shell\open\command`)
		_ = registry.DeleteKey(registry.CURRENT_USER, path+`\shell\open`)
		_ = registry.DeleteKey(registry.CURRENT_USER, path+`\shell`)
		_ = registry.DeleteKey(registry.CURRENT_USER, path)
	})
	if _, err := RegisterScheme(scheme, `C:\k\katyoti.exe`); err != nil {
		t.Fatal(err)
	}
	k, err := registry.OpenKey(registry.CURRENT_USER, path, registry.QUERY_VALUE)
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	if _, _, err := k.GetStringValue("URL Protocol"); err != nil {
		t.Errorf("no URL Protocol value: %v", err)
	}
	c, err := registry.OpenKey(registry.CURRENT_USER, path+`\shell\open\command`, registry.QUERY_VALUE)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	got, _, err := c.GetStringValue("")
	if want := `"C:\k\katyoti.exe" gui --open "%1"`; err != nil || got != want {
		t.Errorf("open command = %q, %v; want %q", got, err, want)
	}
}
