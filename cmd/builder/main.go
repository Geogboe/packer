// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: BUSL-1.1

// Builder - A stateful, idempotent wrapper around Packer
package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"runtime"

	"github.com/hashicorp/go-uuid"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer/command"
	buildercommand "github.com/hashicorp/packer/internal/buildercommand"
	"github.com/hashicorp/packer/packer"
	"github.com/hashicorp/packer/version"
	"github.com/mitchellh/cli"
)

const (
	ErrorPrefix  = "e:"
	OutputPrefix = "o:"
)

var CommandMeta *command.Meta

func main() {
	os.Exit(realMain())
}

func realMain() int {
	// Set GOMAXPROCS if not set
	if os.Getenv("GOMAXPROCS") == "" {
		runtime.GOMAXPROCS(runtime.NumCPU())
	}

	// Setup logging
	packersdk.LogSecretFilter.SetOutput(os.Stderr)
	log.SetOutput(&packersdk.LogSecretFilter)

	log.Printf("[INFO] Builder version: %s (based on Packer %s) [%s %s %s]",
		version.FormattedVersion(),
		version.Version,
		runtime.Version(),
		runtime.GOOS, runtime.GOARCH)

	// Generate UUID for this run
	UUID, _ := uuid.GenerateUUID()
	os.Setenv("PACKER_RUN_UUID", UUID)

	// Load Packer config (for plugin paths, etc)
	config, err := loadPackerConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading configuration: %s\n", err)
		return 1
	}

	// Setup cache directory
	cacheDir, err := packersdk.CachePath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error preparing cache directory: %s\n", err)
		return 1
	}
	log.Printf("[INFO] Setting cache directory: %s", cacheDir)

	// Check for machine-readable mode
	args, machineReadable := extractMachineReadable(os.Args[1:])

	// Cleanup plugins on exit
	defer packer.CleanupClients()

	// Setup UI
	var ui packersdk.Ui
	if machineReadable {
		ui = &packer.MachineReadableUi{
			Writer: os.Stdout,
		}
		os.Setenv("PACKER_NO_COLOR", "1")
	} else {
		ui = &packersdk.BasicUi{
			Reader:      os.Stdin,
			Writer:      os.Stdout,
			ErrorWriter: os.Stdout,
			PB:          &packersdk.NoopProgressTracker{},
		}
	}

	// Create command meta (shared across all commands)
	CommandMeta = &command.Meta{
		CoreConfig: &packer.CoreConfig{
			Components: packer.ComponentFinder{
				PluginConfig: config.Plugins,
			},
			Version: version.Version,
		},
		Ui: ui,
	}

	// Setup CLI
	cli := &cli.CLI{
		Args:         args,
		Autocomplete: true,
		Commands:     Commands(),
		HelpFunc:     cli.BasicHelpFunc("builder"),
		HelpWriter:   os.Stdout,
		Name:         "builder",
		Version:      version.Version,
	}

	exitCode, err := cli.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error executing CLI: %s\n", err)
		return 1
	}

	return exitCode
}

// Commands returns the command mapping for builder CLI
func Commands() map[string]cli.CommandFactory {
	return map[string]cli.CommandFactory{
		// Our enhanced stateful build command
		"build": func() (cli.Command, error) {
			return &buildercommand.BuildCommand{Meta: *CommandMeta}, nil
		},

		// State management commands
		"state": func() (cli.Command, error) {
			return &buildercommand.StateCommand{Meta: *CommandMeta}, nil
		},
		"state show": func() (cli.Command, error) {
			return &buildercommand.StateShowCommand{Meta: *CommandMeta}, nil
		},
		"state rm": func() (cli.Command, error) {
			return &buildercommand.StateRmCommand{Meta: *CommandMeta}, nil
		},

		// Pass through other Packer commands
		"validate": func() (cli.Command, error) {
			return &command.ValidateCommand{Meta: *CommandMeta}, nil
		},
		"init": func() (cli.Command, error) {
			return &command.InitCommand{Meta: *CommandMeta}, nil
		},
		"fmt": func() (cli.Command, error) {
			return &command.FormatCommand{Meta: *CommandMeta}, nil
		},
		"inspect": func() (cli.Command, error) {
			return &command.InspectCommand{Meta: *CommandMeta}, nil
		},
		"console": func() (cli.Command, error) {
			return &command.ConsoleCommand{Meta: *CommandMeta}, nil
		},
		"version": func() (cli.Command, error) {
			return &command.VersionCommand{Meta: *CommandMeta}, nil
		},
	}
}

// extractMachineReadable checks args for -machine-readable flag
func extractMachineReadable(args []string) ([]string, bool) {
	for i, arg := range args {
		if arg == "-machine-readable" {
			result := make([]string, len(args)-1)
			copy(result, args[:i])
			copy(result[i:], args[i+1:])
			return result, true
		}
	}
	return args, false
}

// config represents the Packer configuration
type config struct {
	DisableCheckpoint          bool `json:"disable_checkpoint"`
	DisableCheckpointSignature bool `json:"disable_checkpoint_signature"`
	Plugins                    *packer.PluginConfig
}

// loadPackerConfig loads the Packer configuration
func loadPackerConfig() (*config, error) {
	pluginDir, err := packer.PluginFolder()
	if err != nil {
		return nil, err
	}

	config := &config{
		Plugins: &packer.PluginConfig{
			PluginMinPort:   10000,
			PluginMaxPort:   25000,
			PluginDirectory: pluginDir,
			Builders:        packer.MapOfBuilder{},
			Provisioners:    packer.MapOfProvisioner{},
			PostProcessors:  packer.MapOfPostProcessor{},
			DataSources:     packer.MapOfDatasource{},
		},
	}

	// Load plugins from default location
	if err := config.Plugins.Discover(); err != nil {
		log.Printf("[WARN] Error discovering plugins: %s", err)
	}

	return config, nil
}
