// SPDX-License-Identifier: Apache-2.0

package core

import "github.com/go-toast/toast"

// pushDesktopNotification raises a Windows toast via the Action Center.
func pushDesktopNotification(title, message string) error {
	notification := toast.Notification{
		AppID:   "ktw-notify",
		Title:   title,
		Message: message,
	}
	return notification.Push()
}
