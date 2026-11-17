// SPDX-License-Identifier: Apache-2.0

// Command ktw-mcp is the KaTyoti MCP server used like the command line: it
// speaks MCP over stdio and serves one tool per program of the installation it
// belongs to (ktw-agents, katyoti, ktw-compose), each taking the command line's
// arguments and running that installation's binary per call.
package main

import (
	"fmt"
	"os"

	"github.com/katyoti/katyoti-fwk/app/ktw-mcp/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
