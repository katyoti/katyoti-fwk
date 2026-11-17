// SPDX-License-Identifier: Apache-2.0

// Command gen writes the TaskService bus client (go generate in ../).
package main

import (
	"fmt"
	"os"

	"github.com/katyoti/katyoti-fwk/internal/appcore/busclient"
	"github.com/katyoti/katyoti-fwk/internal/extensions/task/remote/internal/genspec"
)

func main() {
	src, err := busclient.Generate(genspec.Spec())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.WriteFile(genspec.FileName, src, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
