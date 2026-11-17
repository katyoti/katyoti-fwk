// SPDX-License-Identifier: Apache-2.0

package appcore

import (
	"github.com/katyoti/katyoti-fwk/internal/core/console"
	"github.com/spf13/cobra"
)

// NewVersionCommand builds a "version" cobra command.
func NewVersionCommand(appName, version string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Show " + appName + " version",
	}

	mode := console.AddOutputFlag(cmd)

	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		v := struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		}{Name: appName, Version: version}
		return console.RenderConsole(*mode, "version", v, console.RenderFunc(func(p *console.Printer, val any) error {
			cmd.Printf("%s %s\n", appName, version)
			return nil
		}))
	}

	return cmd
}

// ShowVersion prints the version to stdout
// from rclone : need import of https://github.com/rclone/rclone/tree/master/lib/buildinfo

// 	osVersion, osKernel := buildinfo.GetOSVersion()
// 	if osVersion == "" {
// 		osVersion = "unknown"
// 	}
// 	if osKernel == "" {
// 		osKernel = "unknown"
// 	}

// 	linking, tagString := buildinfo.GetLinkingAndTags()

// 	arch := buildinfo.GetArch()

// 	fmt.Printf("rclone %s\n", fs.Version)
// 	fmt.Printf("- os/version: %s\n", osVersion)
// 	fmt.Printf("- os/kernel: %s\n", osKernel)
// 	fmt.Printf("- os/type: %s\n", runtime.GOOS)
// 	fmt.Printf("- os/arch: %s\n", arch)
// 	fmt.Printf("- go/version: %s\n", runtime.Version())
// 	fmt.Printf("- go/linking: %s\n", linking)
// 	fmt.Printf("- go/tags: %s\n", tagString)
// }
