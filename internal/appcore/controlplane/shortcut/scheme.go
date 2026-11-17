// SPDX-License-Identifier: Apache-2.0

package shortcut

import "fmt"

// LinkScheme is the scheme of an element's link, `katyoti://open/<address>`
// : the launcher registers it with the shortcuts, so a link from
// a log, the Inbox or a chat opens the GUI on that element.
const LinkScheme = "katyoti"

// linkCommand is what a link runs: the installation's katyoti, `gui --open`
// with the link as it came (ktw-gui reads the address out of it).
func linkCommand(target, placeholder string) string {
	return fmt.Sprintf(`"%s" gui --open "%s"`, target, placeholder)
}
