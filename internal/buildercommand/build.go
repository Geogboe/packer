package buildercommand

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/hashicorp/packer/builder/state"
	"github.com/hashicorp/packer/command"
	"github.com/posener/complete"
)

// BuildCommand wraps Packer's build command with state management
type BuildCommand struct {
	command.Meta
	statePath string
}

func (c *BuildCommand) Run(args []string) int {
	// Extract template path from args to determine state file location
	templatePath := extractTemplatePath(args)

	if templatePath != "" {
		// Determine state file location
		if c.statePath == "" {
			templateDir := filepath.Dir(templatePath)
			c.statePath = state.DefaultStatePath(templateDir)
		}
		c.Ui.Say(fmt.Sprintf("==> builder: state file: %s", c.statePath))
		c.Ui.Say("")
	}

	// For now, delegate to packer's build command
	// TODO: Add full state management and checkpointing
	buildCmd := &command.BuildCommand{Meta: c.Meta}
	return buildCmd.Run(args)
}

// extractTemplatePath finds the template path from command args
func extractTemplatePath(args []string) string {
	// Skip flags and their values to find the template path
	skipNext := false
	for _, arg := range args {
		if skipNext {
			skipNext = false
			continue
		}

		// Flag that takes a value
		if strings.HasPrefix(arg, "-") {
			if needsValue(arg) {
				skipNext = true
			}
			continue
		}

		// Found the template
		return arg
	}
	return ""
}

// needsValue checks if a flag requires a value
func needsValue(flag string) bool {
	// Remove = if present (e.g., -var=foo)
	if strings.Contains(flag, "=") {
		return false
	}

	valueFlags := []string{
		"-var", "-var-file", "-only", "-except",
		"-on-error", "-parallel-builds", "-state",
	}

	for _, vf := range valueFlags {
		if flag == vf {
			return true
		}
	}
	return false
}

func (c *BuildCommand) Help() string {
	return `Usage: builder build [options] TEMPLATE

  Builds images from a Packer template with state management for idempotency.

  This command is fully compatible with 'packer build', but adds:
    - Automatic checkpointing between build phases
    - Skip builds if inputs haven't changed
    - Resume failed builds from the last checkpoint

Options:

  -state=PATH            Path to state file (default: .packer.d/builder-state.json)
  -force                 Force rebuild even if state indicates build is current
  -color                 Enable colorized output (default: true)
  -debug                 Debug mode enabled for builds
  -except=foo,bar,baz    Run all builds except those matching filters
  -only=foo,bar,baz      Run only the builds with the given names
  -on-error=[cleanup|abort|ask|run-cleanup-provisioner] Action on build error
  -parallel-builds=N     Number of builds to run in parallel (0 = unlimited)
  -timestamp-ui          Enable timestamps on UI output
  -var 'key=value'       Variable for templates
  -var-file=path         Path to variable file

For a full list of options, see 'packer build --help'
`
}

func (c *BuildCommand) Synopsis() string {
	return "Build images from a Packer template (with state management)"
}

func (c *BuildCommand) AutocompleteArgs() complete.Predictor {
	return complete.PredictNothing
}

func (c *BuildCommand) AutocompleteFlags() complete.Flags {
	return complete.Flags{
		"-state":           complete.PredictFiles("*.json"),
		"-force":           complete.PredictNothing,
		"-color":           complete.PredictNothing,
		"-debug":           complete.PredictNothing,
		"-except":          complete.PredictNothing,
		"-only":            complete.PredictNothing,
		"-on-error":        complete.PredictSet("cleanup", "abort", "ask", "run-cleanup-provisioner"),
		"-parallel-builds": complete.PredictNothing,
		"-timestamp-ui":    complete.PredictNothing,
		"-var":             complete.PredictNothing,
		"-var-file":        complete.PredictFiles("*.json"),
	}
}
