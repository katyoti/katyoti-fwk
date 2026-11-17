// SPDX-License-Identifier: Apache-2.0

// io.go defines the transport-agnostic I/O model for task execution.
// IOConfig encapsulates input source, output destination, input parameters,
// and display options. Each transport (CLI pipe, TUI, future MCP server)
// creates its own IOConfig to feed the engine.

package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// InputMode describes where task input comes from.
type InputMode int

const (
	InputNone      InputMode = iota // No input provided (interactive mode)
	InputFile                       // --if <file>
	InputClipboard                  // --ic
	InputStdin                      // --is (or piped stdin)
)

// OutputMode describes where task output goes.
type OutputMode int

const (
	OutputStdout OutputMode = iota // --os (default)
	OutputFile                     // --of <file>
)

// IOConfig holds the resolved I/O settings for a task run.
// It is transport-agnostic: CLI, TUI, and future MCP server mode
// all produce an IOConfig before calling the engine.
type IOConfig struct {
	// Input
	InputMode InputMode
	InputFile string // Set when InputMode == InputFile
	InputData string // Resolved input content (read at resolve time)

	// Output
	OutputMode OutputMode
	OutputFile string // Set when OutputMode == OutputFile
	Raw        bool   // --raw: no Glamour styling, no colors

	// Input parameters
	TaskInput map[string]any // Merged task input: input metadata + --input params

	// Derived
	IsPipe bool // --pipe shortcut was used
}

// ResolveIO builds an IOConfig from CLI flags.
// It reads input data, parses context, and merges everything together.
func ResolveIO(inputFile, inputArg string, inputClipboard, inputStdin, outputStdout, outputFile bool, outputFilePath string, raw, pipe bool) (*IOConfig, error) {
	cfg := &IOConfig{
		TaskInput: make(map[string]any),
	}

	// --pipe shortcut
	if pipe {
		inputStdin = true
		outputStdout = true
		raw = true
		cfg.IsPipe = true
	}

	cfg.Raw = raw

	// Resolve input mode
	exclusiveCount := 0
	if inputFile != "" {
		exclusiveCount++
	}
	if inputClipboard {
		exclusiveCount++
	}
	if inputStdin {
		exclusiveCount++
	}
	if exclusiveCount > 1 {
		return nil, fmt.Errorf("input flags --if, --ic, --is are mutually exclusive")
	}

	switch {
	case inputFile != "":
		cfg.InputMode = InputFile
		cfg.InputFile = inputFile
		data, err := os.ReadFile(inputFile)
		if err != nil {
			return nil, fmt.Errorf("read input file %q: %w", inputFile, err)
		}
		cfg.InputData = string(data)
		cfg.TaskInput["input_file"] = filepath.Base(inputFile)
		cfg.TaskInput["input_file_path"] = inputFile
		cfg.TaskInput["input_content"] = cfg.InputData

	case inputClipboard:
		cfg.InputMode = InputClipboard
		// Clipboard reading is platform-dependent — defer to a helper.
		// For now, try to read from clipboard via OS tools.
		content, err := readClipboard()
		if err != nil {
			return nil, fmt.Errorf("read clipboard: %w", err)
		}
		cfg.InputData = content
		cfg.TaskInput["input_source"] = "clipboard"
		cfg.TaskInput["input_content"] = cfg.InputData

	case inputStdin:
		cfg.InputMode = InputStdin
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("read stdin: %w", err)
		}
		cfg.InputData = string(data)
		cfg.TaskInput["input_source"] = "stdin"
		cfg.TaskInput["input_content"] = cfg.InputData

	default:
		cfg.InputMode = InputNone
	}

	// Resolve output mode
	if outputFile || outputFilePath != "" {
		cfg.OutputMode = OutputFile
		cfg.OutputFile = outputFilePath
	} else {
		cfg.OutputMode = OutputStdout
	}

	// Resolve --input
	if inputArg != "" {
		inputData, err := resolveInput(inputArg, cfg.InputMode == InputStdin)
		if err != nil {
			return nil, fmt.Errorf("resolve input: %w", err)
		}
		// Merge user input params into task input (params override input metadata)
		for k, v := range inputData {
			cfg.TaskInput[k] = v
		}
	}

	return cfg, nil
}

// resolveInput parses the --input argument (the task's input parameters).
// It can be:
//   - "-" or empty: read JSON/YAML from stdin (if not already consumed by --is)
//   - Inline JSON: starts with '{'
//   - A file path ending in .json or .yaml/.yml
//   - Any other file: try JSON then YAML
func resolveInput(arg string, stdinConsumed bool) (map[string]any, error) {
	arg = strings.TrimSpace(arg)

	// Stdin: "--input -" or "--input" with empty value
	if arg == "-" || arg == "" {
		if stdinConsumed {
			return nil, fmt.Errorf("cannot read input from stdin: stdin already consumed by --is")
		}
		raw, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("read input from stdin: %w", err)
		}
		return parseInputBytes(raw, "stdin")
	}

	// Inline JSON
	if strings.HasPrefix(arg, "{") {
		var data map[string]any
		if err := json.Unmarshal([]byte(arg), &data); err != nil {
			return nil, fmt.Errorf("parse inline JSON input: %w", err)
		}
		return data, nil
	}

	// File path with recognized extension
	ext := strings.ToLower(filepath.Ext(arg))
	switch ext {
	case ".json":
		raw, err := os.ReadFile(arg)
		if err != nil {
			return nil, fmt.Errorf("read input file %q: %w", arg, err)
		}
		var data map[string]any
		if err := json.Unmarshal(raw, &data); err != nil {
			return nil, fmt.Errorf("parse JSON input file: %w", err)
		}
		return data, nil

	case ".yaml", ".yml":
		raw, err := os.ReadFile(arg)
		if err != nil {
			return nil, fmt.Errorf("read input file %q: %w", arg, err)
		}
		var data map[string]any
		if err := yaml.Unmarshal(raw, &data); err != nil {
			return nil, fmt.Errorf("parse YAML input file: %w", err)
		}
		return data, nil
	}

	// No recognized extension — try as a generic file
	if _, err := os.Stat(arg); err == nil {
		raw, err := os.ReadFile(arg)
		if err != nil {
			return nil, fmt.Errorf("read input file %q: %w", arg, err)
		}
		return parseInputBytes(raw, arg)
	}

	return nil, fmt.Errorf("input argument %q is not valid JSON and file does not exist", arg)
}

// parseInputBytes tries JSON then YAML on raw bytes.
func parseInputBytes(raw []byte, source string) (map[string]any, error) {
	raw = []byte(strings.TrimSpace(string(raw)))
	if len(raw) == 0 {
		return nil, fmt.Errorf("input from %s is empty", source)
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err == nil {
		return data, nil
	}
	if err := yaml.Unmarshal(raw, &data); err == nil && data != nil {
		return data, nil
	}
	return nil, fmt.Errorf("input from %s is not valid JSON or YAML", source)
}

// readClipboard reads text from the system clipboard.
// Uses OS-specific tools as a lightweight approach.
func readClipboard() (string, error) {
	// Platform-specific clipboard reading.
	// On Windows: powershell Get-Clipboard
	// On macOS: pbpaste
	// On Linux: xclip -selection clipboard -o
	//
	// For now, we use a simple exec-based approach.
	// A proper implementation would use a clipboard library,
	// but we avoid the dependency for now.
	return "", fmt.Errorf("clipboard input not yet implemented (use --if or --is instead)")
}

// WriteOutput writes content to the configured output destination.
func (cfg *IOConfig) WriteOutput(content string) error {
	switch cfg.OutputMode {
	case OutputFile:
		if cfg.OutputFile == "" {
			return fmt.Errorf("output file path not specified")
		}
		dir := filepath.Dir(cfg.OutputFile)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create output directory: %w", err)
		}
		return os.WriteFile(cfg.OutputFile, []byte(content), 0o644)
	case OutputStdout:
		_, err := fmt.Print(content)
		return err
	default:
		return fmt.Errorf("unknown output mode: %d", cfg.OutputMode)
	}
}
