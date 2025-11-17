package buildercommand

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer/builder/state"
	"github.com/hashicorp/packer/builder/wrapper"
	"github.com/hashicorp/packer/command"
	"github.com/hashicorp/packer/packer"
)

// BuildCommand wraps Packer's build command with state management
type BuildCommand struct {
	command.Meta
	statePath string
}

func (c *BuildCommand) Run(args []string) int {
	ctx, cleanup := command.HandleTermInterrupt(c.Ui)
	defer cleanup()

	// Parse build args using Packer's parser
	buildCmd := &command.BuildCommand{Meta: c.Meta}
	cfg, ret := buildCmd.ParseArgs(args)
	if ret != 0 {
		return ret
	}

	// Determine state file location
	if c.statePath == "" {
		templateDir := filepath.Dir(cfg.Path)
		c.statePath = state.DefaultStatePath(templateDir)
	}

	c.Ui.Say(fmt.Sprintf("Builder: Using state file: %s", c.statePath))

	// Load and lock state
	stateManager := state.NewManager(c.statePath)
	st, err := stateManager.Load()
	if err != nil {
		c.Ui.Error(fmt.Sprintf("Failed to load state: %s", err))
		return 1
	}
	defer func() {
		if err := stateManager.Close(); err != nil {
			log.Printf("[WARN] Failed to close state: %s", err)
		}
	}()

	// Compute template hash for change detection
	templateHash, err := state.ComputeFileHash(cfg.Path)
	if err != nil {
		c.Ui.Error(fmt.Sprintf("Failed to hash template: %s", err))
		return 1
	}

	// Check if inputs have changed
	variables := cfg.Vars // Assuming BuildArgs has Vars field
	inputsChanged := stateManager.InputsChanged(templateHash, variables, make(map[string]string))

	if !inputsChanged {
		c.Ui.Say("✓ Template inputs unchanged")

		// Check if all builds are complete
		allComplete := true
		for buildName, build := range st.Builds {
			if !build.IsComplete() {
				allComplete = false
				c.Ui.Say(fmt.Sprintf("Build '%s' incomplete, will resume", buildName))
			} else {
				c.Ui.Say(fmt.Sprintf("Build '%s' already complete", buildName))
			}
		}

		if allComplete && len(st.Builds) > 0 {
			c.Ui.Say("\n✓ All builds complete and inputs unchanged. Nothing to do!")
			c.Ui.Say("Use -force to rebuild anyway.")
			return 0
		}
	} else {
		c.Ui.Say("Template inputs changed, will rebuild")
	}

	// Update state with current inputs
	stateManager.UpdateTemplateInputs(cfg.Path, templateHash, variables, make(map[string]string))

	// Run the build with our stateful wrapper
	return c.runStatefulBuild(ctx, cfg, stateManager)
}

func (c *BuildCommand) runStatefulBuild(ctx context.Context, cfg *command.BuildArgs, stateManager *state.Manager) int {
	// Initialize Packer core config
	c.CoreConfig.Components.PluginConfig.ReleasesOnly = cfg.ReleaseOnly

	packerStarter, ret := c.GetConfig(&cfg.MetaArgs)
	if ret != 0 {
		return ret
	}

	// Detect and initialize plugins
	diags := packerStarter.DetectPluginBinaries()
	if writeDiagsRet := command.WriteDiags(c.Ui, nil, diags); writeDiagsRet != 0 {
		return writeDiagsRet
	}

	diags = packerStarter.Initialize(packer.InitializeOptions{
		UseSequential: cfg.UseSequential,
	})
	if writeDiagsRet := command.WriteDiags(c.Ui, nil, diags); writeDiagsRet != 0 {
		return writeDiagsRet
	}

	// Get builds
	builds, diags := packerStarter.GetBuilds(packer.GetBuildsOptions{
		Only:    cfg.Only,
		Except:  cfg.Except,
		Debug:   cfg.Debug,
		Force:   cfg.Force,
		OnError: cfg.OnError,
	})

	ret = command.WriteDiags(c.Ui, nil, diags)
	if len(builds) == 0 && ret != 0 {
		return ret
	}

	if len(builds) == 0 {
		c.Ui.Error("No builds found in template")
		return 1
	}

	c.Ui.Say(fmt.Sprintf("Found %d build(s) to run", len(builds)))

	// Wrap each build with our stateful wrapper
	var artifacts []packersdk.Artifact
	for _, coreBuild := range builds {
		c.Ui.Say(fmt.Sprintf("\n==> %s: Starting build", coreBuild.Name()))

		statefulBuild := wrapper.NewStatefulBuild(coreBuild, stateManager)
		buildArtifacts, err := statefulBuild.Run(ctx, c.Ui)

		if err != nil {
			c.Ui.Error(fmt.Sprintf("Build '%s' failed: %s", coreBuild.Name(), err))
			return 1
		}

		artifacts = append(artifacts, buildArtifacts...)
	}

	// Print summary
	c.Ui.Say(fmt.Sprintf("\n==> Builds finished. The artifacts were:"))
	for _, artifact := range artifacts {
		c.Ui.Say(fmt.Sprintf("    %s: %s", artifact.BuilderId(), artifact.String()))
	}

	return 0
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
