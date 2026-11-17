// SPDX-License-Identifier: Apache-2.0

// Package cmd is ktw-mcp's entry: it reads its flags and serves MCP on stdin
// and stdout until stdin closes.
package cmd

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/katyoti/katyoti-fwk/app/ktw-mcp/internal/server"
)

// version is stamped at build time (-X .../app/ktw-mcp/cmd.version=...).
var version = "dev"

// Execute serves MCP over stdio. The installation is the folder holding this
// executable: the programs run and the allow-list read are the ones beside it.
func Execute() error {
	fs := flag.NewFlagSet("ktw-mcp", flag.ContinueOnError)
	timeout := fs.Duration("timeout", 10*time.Minute, "the longest one command may run")
	showVersion := fs.Bool("version", false, "print the version and exit")
	if err := fs.Parse(os.Args[1:]); err != nil {
		return err
	}
	if *showVersion {
		fmt.Println(version)
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("ktw-mcp: where am I: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	srv, err := server.New(server.Options{
		Dir:     filepath.Dir(exe),
		Version: version,
		Timeout: *timeout,
	})
	if err != nil {
		return err
	}
	return srv.Serve(os.Stdin, os.Stdout)
}
