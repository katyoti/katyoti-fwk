// SPDX-License-Identifier: Apache-2.0

package humangate

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"

	hgapi "github.com/katyoti/katyoti-fwk/internal/extensions/humangate/api"
)

// CLIPrompter implements HumanGate for pipe mode (raw stdin/stdout).
// Supports text, select, confirm prompts. No form support (pipe mode only).
type CLIPrompter struct {
	reader *bufio.Reader
	writer io.Writer
}

// NewCLIPrompter creates a prompter that reads from reader and writes to writer.
func NewCLIPrompter(reader io.Reader, writer io.Writer) *CLIPrompter {
	return &CLIPrompter{
		reader: bufio.NewReader(reader),
		writer: writer,
	}
}

func (p *CLIPrompter) Prompt(ctx context.Context, in hgapi.PromptRequest) (hgapi.PromptResponse, error) {
	switch in.Type {
	case "text", "":
		fmt.Fprintf(p.writer, "\n%s\n> ", in.Question)
		line, err := p.reader.ReadString('\n')
		if err != nil {
			return hgapi.PromptResponse{}, err
		}
		return hgapi.PromptResponse{Answer: strings.TrimSpace(line)}, nil

	case "select":
		fmt.Fprintf(p.writer, "\n%s\n", in.Question)
		for i, opt := range in.Options {
			fmt.Fprintf(p.writer, "  %d. %s\n", i+1, opt)
		}
		fmt.Fprint(p.writer, "Enter number: ")
		line, err := p.reader.ReadString('\n')
		if err != nil {
			return hgapi.PromptResponse{}, err
		}
		line = strings.TrimSpace(line)
		var idx int
		if _, err := fmt.Sscanf(line, "%d", &idx); err != nil || idx < 1 || idx > len(in.Options) {
			return hgapi.PromptResponse{}, fmt.Errorf("invalid selection: %s", line)
		}
		return hgapi.PromptResponse{Answer: in.Options[idx-1]}, nil

	case "confirm":
		fmt.Fprintf(p.writer, "\n%s [y/N]: ", in.Question)
		line, err := p.reader.ReadString('\n')
		if err != nil {
			return hgapi.PromptResponse{}, err
		}
		line = strings.TrimSpace(strings.ToLower(line))
		return hgapi.PromptResponse{Confirmed: line == "y" || line == "yes"}, nil

	case "form":
		return hgapi.PromptResponse{}, fmt.Errorf("form prompts not supported in pipe mode")

	default:
		return hgapi.PromptResponse{}, fmt.Errorf("unknown prompt type: %s", in.Type)
	}
}

func (p *CLIPrompter) Capabilities() []string {
	return []string{"text", "select", "confirm"}
}

// PromptPermission renders a deny-first numbered menu for the kernel
// permission gate.
func (p *CLIPrompter) PromptPermission(ctx context.Context, in hgapi.PromptPermissionRequest) (hgapi.PromptPermissionResponse, error) {
	fmt.Fprintf(p.writer, "\n%s\n", in.Question)
	fmt.Fprintln(p.writer, "  1. Deny once  (default)")
	fmt.Fprintln(p.writer, "  2. Allow once")

	for i, sug := range in.Suggestions {
		fmt.Fprintf(p.writer, "  %d. %s\n", 3+i, sug.Label)
	}

	denyAlwaysSlot := 3 + len(in.Suggestions)
	fmt.Fprintf(p.writer, "  %d. Never allow tool %q (deny always)\n", denyAlwaysSlot, in.ToolName)

	fmt.Fprint(p.writer, "Choose [1]: ")
	line, err := p.reader.ReadString('\n')
	if err != nil {
		return hgapi.PromptPermissionResponse{Decision: hgapi.PermissionDecisionDenyOnce}, err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return hgapi.PromptPermissionResponse{Decision: hgapi.PermissionDecisionDenyOnce}, nil
	}

	var choice int
	if _, err := fmt.Sscanf(line, "%d", &choice); err != nil {
		return hgapi.PromptPermissionResponse{Decision: hgapi.PermissionDecisionDenyOnce},
			fmt.Errorf("invalid permission choice %q", line)
	}

	switch {
	case choice == 1:
		return hgapi.PromptPermissionResponse{Decision: hgapi.PermissionDecisionDenyOnce}, nil
	case choice == 2:
		return hgapi.PromptPermissionResponse{Decision: hgapi.PermissionDecisionAllowOnce}, nil
	case choice >= 3 && choice <= 2+len(in.Suggestions):
		sug := in.Suggestions[choice-3]
		return hgapi.PromptPermissionResponse{
			Decision:     hgapi.PermissionDecisionAllowAlways,
			SelectedRule: sug.Rule,
		}, nil
	case choice == denyAlwaysSlot:
		return hgapi.PromptPermissionResponse{
			Decision:     hgapi.PermissionDecisionDenyAlways,
			SelectedRule: in.ToolName + ":**",
		}, nil
	default:
		return hgapi.PromptPermissionResponse{Decision: hgapi.PermissionDecisionDenyOnce},
			fmt.Errorf("permission choice %d out of range (1..%d)", choice, denyAlwaysSlot)
	}
}
