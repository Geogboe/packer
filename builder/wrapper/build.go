package wrapper

import (
	"context"
	"fmt"
	"log"
	"time"

	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer/builder/state"
	"github.com/hashicorp/packer/packer"
)

// StatefulBuild wraps a CoreBuild to add state management and checkpointing
type StatefulBuild struct {
	inner        *packer.CoreBuild
	stateManager *state.Manager
	buildName    string
}

// NewStatefulBuild creates a new stateful build wrapper
func NewStatefulBuild(coreBuild *packer.CoreBuild, stateManager *state.Manager) *StatefulBuild {
	return &StatefulBuild{
		inner:        coreBuild,
		stateManager: stateManager,
		buildName:    coreBuild.BuildName,
	}
}

// Run executes the build with state management and checkpointing
func (sb *StatefulBuild) Run(ctx context.Context, ui packersdk.Ui) ([]packersdk.Artifact, error) {
	st := sb.stateManager.State()
	if st == nil {
		return nil, fmt.Errorf("state not loaded")
	}

	buildState := st.GetBuild(sb.buildName)

	// Check if build is already complete and inputs haven't changed
	if buildState != nil && buildState.IsComplete() {
		ui.Say(fmt.Sprintf("Build '%s' already complete, checking if rebuild needed...", sb.buildName))

		// If inputs haven't changed, return cached artifacts
		if !sb.inputsChangedSinceLastBuild() {
			ui.Say(fmt.Sprintf("✓ Build '%s' is up-to-date, using existing artifacts", sb.buildName))
			return sb.loadArtifactsFromState(buildState)
		}

		ui.Say("Inputs changed, rebuilding...")
		buildState = nil // Start fresh
	}

	// Initialize build state if needed
	if buildState == nil {
		buildState = &state.Build{
			Name:         sb.buildName,
			Type:         sb.inner.BuilderType,
			Status:       state.BuildStatusPending,
			Provisioners: make([]state.ProvisionerState, len(sb.inner.Provisioners)),
			StartedAt:    time.Now(),
		}

		// Initialize provisioner states
		for i, p := range sb.inner.Provisioners {
			buildState.Provisioners[i] = state.ProvisionerState{
				Type:   p.PType,
				Status: state.StatusPending,
			}
		}

		st.SetBuild(sb.buildName, buildState)
		if err := sb.stateManager.Save(); err != nil {
			return nil, fmt.Errorf("failed to save initial state: %w", err)
		}
	}

	// Check if we have an existing instance to resume
	if buildState.HasInstance() {
		ui.Say(fmt.Sprintf("Found existing instance: %s", buildState.Instance.ID))
		ui.Say("Attempting to resume from checkpoint...")

		artifacts, err := sb.resumeBuild(ctx, ui, buildState)
		if err != nil {
			// If resume fails, clean up and start over
			ui.Error(fmt.Sprintf("Failed to resume: %s", err))
			ui.Say("Starting fresh build...")
			buildState.Instance = nil
			buildState.Status = state.BuildStatusPending
		} else {
			return artifacts, nil
		}
	}

	// Run the normal build flow
	return sb.runFreshBuild(ctx, ui, buildState)
}

// runFreshBuild executes a build from scratch
func (sb *StatefulBuild) runFreshBuild(ctx context.Context, ui packersdk.Ui, buildState *state.Build) ([]packersdk.Artifact, error) {
	st := sb.stateManager.State()

	// Update status
	buildState.Status = state.BuildStatusCreating
	if err := sb.stateManager.Save(); err != nil {
		return nil, err
	}

	// Run the builder (this creates VM and runs provisioners via hooks)
	// NOTE: In future, we want to intercept provisioning to checkpoint between them
	// For now, we let the builder run completely, then checkpoint

	ui.Say(fmt.Sprintf("Running builder: %s", sb.inner.BuilderType))

	// Call the original CoreBuild.Run()
	artifacts, err := sb.inner.Run(ctx, ui)

	if err != nil {
		// Build failed
		buildState.Status = state.BuildStatusFailed
		buildState.Error = err.Error()
		st.SetBuild(sb.buildName, buildState)
		sb.stateManager.Save()
		return nil, err
	}

	// Build succeeded!
	buildState.Status = state.BuildStatusComplete
	buildState.CompletedAt = time.Now()

	// Store artifacts in state
	buildState.Artifacts = sb.artifactsToState(artifacts)

	st.SetBuild(sb.buildName, buildState)
	if err := sb.stateManager.Save(); err != nil {
		log.Printf("Warning: failed to save completion state: %s", err)
	}

	return artifacts, nil
}

// resumeBuild attempts to resume a build from a checkpoint
func (sb *StatefulBuild) resumeBuild(ctx context.Context, ui packersdk.Ui, buildState *state.Build) ([]packersdk.Artifact, error) {
	// For now, we can't actually resume mid-build because Builder.Run() is atomic
	// This is where we'd implement reconnection logic in the future

	// TODO: Implement reconnection to existing instance
	// TODO: Re-run only pending provisioners
	// TODO: Re-run only pending post-processors

	return nil, fmt.Errorf("resume not yet implemented - builder must complete atomically")
}

// inputsChangedSinceLastBuild checks if inputs have changed since the last successful build
func (sb *StatefulBuild) inputsChangedSinceLastBuild() bool {
	// This will be implemented when we track template/variable changes
	// For now, assume inputs haven't changed if we have a complete build
	return false
}

// loadArtifactsFromState reconstructs artifacts from state
func (sb *StatefulBuild) loadArtifactsFromState(buildState *state.Build) ([]packersdk.Artifact, error) {
	// TODO: Implement artifact reconstruction from state
	// For now, we can't safely return cached artifacts without validation

	artifacts := make([]packersdk.Artifact, len(buildState.Artifacts))
	for i, artState := range buildState.Artifacts {
		artifacts[i] = &CachedArtifact{
			id:        artState.ID,
			builderID: artState.BuilderID,
			files:     artState.Files,
		}
	}

	return artifacts, nil
}

// artifactsToState converts Packer artifacts to state format
func (sb *StatefulBuild) artifactsToState(artifacts []packersdk.Artifact) []state.ArtifactState {
	result := make([]state.ArtifactState, len(artifacts))

	for i, art := range artifacts {
		result[i] = state.ArtifactState{
			ID:        art.Id(),
			BuilderID: art.BuilderId(),
			Files:     art.Files(),
		}
	}

	return result
}

// CachedArtifact represents an artifact loaded from state
type CachedArtifact struct {
	id        string
	builderID string
	files     []string
}

func (a *CachedArtifact) BuilderId() string {
	return a.builderID
}

func (a *CachedArtifact) Files() []string {
	return a.files
}

func (a *CachedArtifact) Id() string {
	return a.id
}

func (a *CachedArtifact) String() string {
	return fmt.Sprintf("Cached artifact: %s", a.id)
}

func (a *CachedArtifact) State(name string) interface{} {
	return nil
}

func (a *CachedArtifact) Destroy() error {
	// Cached artifacts shouldn't be destroyed
	return nil
}
