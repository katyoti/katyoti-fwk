// SPDX-License-Identifier: Apache-2.0

// export.go — exporting a .drawio file to an image through the draw.io
// desktop CLI, the one PNG / SVG / PDF path of the engine.
package render

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	drawioapi "github.com/katyoti/katyoti-fwk/internal/extensions/drawio/api"
)

// FindDrawioExecutable locates the draw.io desktop application executable.
func FindDrawioExecutable() (string, error) {
	for _, name := range []string{"draw.io", "drawio"} {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
	}

	switch runtime.GOOS {
	case "windows":
		candidates := []string{
			filepath.Join(os.Getenv("PROGRAMFILES"), "draw.io", "draw.io.exe"),
			filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "draw.io", "draw.io.exe"),
		}
		for _, c := range candidates {
			if _, err := os.Stat(c); err == nil {
				return c, nil
			}
		}
	case "darwin":
		macPath := "/Applications/draw.io.app/Contents/MacOS/draw.io"
		if _, err := os.Stat(macPath); err == nil {
			return macPath, nil
		}
	}

	return "", &drawioapi.DrawioError{Message: "draw.io executable not found; install draw.io desktop or add it to PATH"}
}

// Export renders a .drawio file to outputFormat ("png", "svg" or "pdf")
// through the draw.io desktop CLI and returns the bytes.
func Export(sourcePath, outputFormat string) ([]byte, error) {
	executable, err := FindDrawioExecutable()
	if err != nil {
		return nil, err
	}
	// The draw.io CLI is picky about relative paths.
	absSource, err := filepath.Abs(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("drawio: resolve %q: %w", sourcePath, err)
	}

	tmpFile, err := os.CreateTemp("", "drawio_*."+outputFormat)
	if err != nil {
		return nil, fmt.Errorf("drawio: create temp file: %w", err)
	}
	target := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(target)

	args := []string{"--export", "--format", outputFormat, "--output", target}
	switch outputFormat {
	case "png":
		// scale=1 is the only safe default: --scale 2 doubles each axis
		// (4x pixels), which makes Electron's V8 crash out of memory on
		// pages wider or taller than about 3000px.
		args = append(args, "--scale", "1", "--transparent")
	case "svg":
		args = append(args, "--embed-svg-images")
	}
	// --no-sandbox is needed on Windows and Linux when run from a
	// non-interactive context.
	if runtime.GOOS != "darwin" {
		args = append(args, "--no-sandbox")
	}
	// No "--" before the path: draw.io's Electron CLI then exports nothing.
	// The path is absolute, so it cannot be read as a flag.
	args = append(args, absSource)

	proc := exec.Command(executable, args...)
	output, err := proc.CombinedOutput()
	if err != nil {
		exitCode := -1
		if proc.ProcessState != nil {
			exitCode = proc.ProcessState.ExitCode()
		}
		msg := fmt.Sprintf("failed to convert draw.io diagram; exit code: %d", exitCode)
		if len(output) > 0 {
			msg += "\noutput:\n" + string(output)
		}
		return nil, &drawioapi.DrawioError{Message: msg}
	}

	data, err := os.ReadFile(target)
	if err != nil {
		return nil, fmt.Errorf("drawio: read output file: %w", err)
	}
	if len(data) == 0 {
		msg := "draw.io produced empty output"
		if len(output) > 0 {
			msg += "; CLI output: " + string(output)
		}
		return nil, &drawioapi.DrawioError{Message: msg}
	}
	return data, nil
}
