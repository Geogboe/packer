package buildercommand

import (
	"github.com/hashicorp/packer/command"
	"github.com/posener/complete"
)

// StateCommand is the parent command for state management
type StateCommand struct {
	command.Meta
}

func (c *StateCommand) Run(args []string) int {
	return c.Meta.CommandRunner.Run(append([]string{"state"}, args...))
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
	c.Ui.Say("State show not yet implemented")
	return 0
}

func (c *StateShowCommand) Help() string {
	return `Usage: builder state show

  Show the current builder state.
`
}

func (c *StateShowCommand) Synopsis() string {
	return "Show current state"
}

func (c *StateShowCommand) AutocompleteArgs() complete.Predictor {
	return complete.PredictNothing
}

func (c *StateShowCommand) AutocompleteFlags() complete.Flags {
	return complete.Flags{}
}

// StateRmCommand removes a build from state
type StateRmCommand struct {
	command.Meta
}

func (c *StateRmCommand) Run(args []string) int {
	c.Ui.Say("State rm not yet implemented")
	return 0
}

func (c *StateRmCommand) Help() string {
	return `Usage: builder state rm BUILD_NAME

  Remove a build from the state file.
`
}

func (c *StateRmCommand) Synopsis() string {
	return "Remove a build from state"
}

func (c *StateRmCommand) AutocompleteArgs() complete.Predictor {
	return complete.PredictNothing
}

func (c *StateRmCommand) AutocompleteFlags() complete.Flags {
	return complete.Flags{}
}
