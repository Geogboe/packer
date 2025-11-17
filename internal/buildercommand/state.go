package buildercommand

import (
	"flag"
	"fmt"

	"github.com/hashicorp/packer/builder/state"
	"github.com/hashicorp/packer/command"
	"github.com/posener/complete"
)

// StateCommand is the parent command for state management
type StateCommand struct {
	command.Meta
}

func (c *StateCommand) Run(args []string) int {
	c.Ui.Error("Usage: builder state <subcommand>\n\nSubcommands:\n  show    Show the current state\n  rm      Remove a build from state")
	return 1
}

func (c *StateCommand) Help() string {
	return `Usage: builder state <subcommand> [options]

  Manage the builder state file.

Subcommands:
    show    Show the current state
    rm      Remove a build from state
`
}

func (c *StateCommand) Synopsis() string {
	return "Manage builder state"
}

func (c *StateCommand) AutocompleteArgs() complete.Predictor {
	return complete.PredictNothing
}

func (c *StateCommand) AutocompleteFlags() complete.Flags {
	return complete.Flags{}
}

// StateShowCommand shows the current state
type StateShowCommand struct {
	command.Meta
}

func (c *StateShowCommand) Run(args []string) int {
	var statePath string

	flags := flag.NewFlagSet("state show", flag.ContinueOnError)
	flags.StringVar(&statePath, "state", "", "Path to state file")
	if err := flags.Parse(args); err != nil {
		return 1
	}

	// Default state path
	if statePath == "" {
		statePath = state.DefaultStatePath(".")
	}

	// Load state
	st, err := state.Load(statePath)
	if err != nil {
		c.Ui.Error(fmt.Sprintf("Error loading state: %s", err))
		return 1
	}

	if st == nil {
		c.Ui.Say("No state file found.")
		return 0
	}

	// Pretty print the state
	c.Ui.Say(fmt.Sprintf("State file: %s", statePath))
	c.Ui.Say(fmt.Sprintf("Version: %d (serial: %d)", st.Version, st.Serial))
	c.Ui.Say(fmt.Sprintf("Template: %s", st.Template.Path))
	c.Ui.Say(fmt.Sprintf("Template Hash: %s", st.Template.Hash))
	c.Ui.Say("")

	if len(st.Builds) == 0 {
		c.Ui.Say("No builds in state.")
		return 0
	}

	c.Ui.Say(fmt.Sprintf("Builds (%d):", len(st.Builds)))
	for name, build := range st.Builds {
		c.Ui.Say(fmt.Sprintf("\n  %s:", name))
		c.Ui.Say(fmt.Sprintf("    Type: %s", build.Type))
		c.Ui.Say(fmt.Sprintf("    Status: %s", build.Status))

		if build.Instance != nil {
			c.Ui.Say(fmt.Sprintf("    Instance:"))
			c.Ui.Say(fmt.Sprintf("      ID: %s", build.Instance.ID))
			if build.Instance.PublicIP != "" {
				c.Ui.Say(fmt.Sprintf("      IP: %s", build.Instance.PublicIP))
			}
			if build.Instance.Provider != "" {
				c.Ui.Say(fmt.Sprintf("      Provider: %s", build.Instance.Provider))
			}
		}

		if len(build.Provisioners) > 0 {
			completedCount := 0
			for _, p := range build.Provisioners {
				if p.Status == state.StatusComplete {
					completedCount++
				}
			}
			c.Ui.Say(fmt.Sprintf("    Provisioners: %d/%d complete", completedCount, len(build.Provisioners)))
		}

		if len(build.Artifacts) > 0 {
			c.Ui.Say(fmt.Sprintf("    Artifacts:"))
			for _, art := range build.Artifacts {
				c.Ui.Say(fmt.Sprintf("      - %s (%s)", art.ID, art.BuilderID))
			}
		}

		if !build.CompletedAt.IsZero() {
			c.Ui.Say(fmt.Sprintf("    Completed: %s", build.CompletedAt.Format("2006-01-02 15:04:05")))
		}
	}

	return 0
}

func (c *StateShowCommand) Help() string {
	return `Usage: builder state show [-state=path]

  Show the current builder state.

Options:
  -state=path    Path to state file (default: .packer.d/builder-state.json)
`
}

func (c *StateShowCommand) Synopsis() string {
	return "Show current state"
}

func (c *StateShowCommand) AutocompleteArgs() complete.Predictor {
	return complete.PredictNothing
}

func (c *StateShowCommand) AutocompleteFlags() complete.Flags {
	return complete.Flags{
		"-state": complete.PredictFiles("*.json"),
	}
}

// StateRmCommand removes a build from state
type StateRmCommand struct {
	command.Meta
}

func (c *StateRmCommand) Run(args []string) int {
	var statePath string

	flags := flag.NewFlagSet("state rm", flag.ContinueOnError)
	flags.StringVar(&statePath, "state", "", "Path to state file")
	if err := flags.Parse(args); err != nil {
		return 1
	}

	args = flags.Args()
	if len(args) != 1 {
		c.Ui.Error("Usage: builder state rm BUILD_NAME")
		return 1
	}

	buildName := args[0]

	// Default state path
	if statePath == "" {
		statePath = state.DefaultStatePath(".")
	}

	// Load state with locking
	manager := state.NewManager(statePath)
	st, err := manager.Load()
	if err != nil {
		c.Ui.Error(fmt.Sprintf("Error loading state: %s", err))
		return 1
	}
	defer manager.Unlock()

	if st == nil {
		c.Ui.Error("No state file found")
		return 1
	}

	// Check if build exists
	if st.GetBuild(buildName) == nil {
		c.Ui.Error(fmt.Sprintf("Build '%s' not found in state", buildName))
		return 1
	}

	// Remove build
	st.RemoveBuild(buildName)

	// Save
	if err := manager.Save(); err != nil {
		c.Ui.Error(fmt.Sprintf("Error saving state: %s", err))
		return 1
	}

	c.Ui.Say(fmt.Sprintf("Removed build '%s' from state", buildName))
	return 0
}

func (c *StateRmCommand) Help() string {
	return `Usage: builder state rm [-state=path] BUILD_NAME

  Remove a build from the state file.

Options:
  -state=path    Path to state file (default: .packer.d/builder-state.json)
`
}

func (c *StateRmCommand) Synopsis() string {
	return "Remove a build from state"
}

func (c *StateRmCommand) AutocompleteArgs() complete.Predictor {
	return complete.PredictNothing
}

func (c *StateRmCommand) AutocompleteFlags() complete.Flags {
	return complete.Flags{
		"-state": complete.PredictFiles("*.json"),
	}
}
