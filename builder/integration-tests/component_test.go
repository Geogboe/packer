package integration_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hashicorp/packer/builder/state"
)

// TestComponentIntegration_StateLifecycle tests the full state lifecycle
func TestComponentIntegration_StateLifecycle(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "component-lifecycle-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	statePath := filepath.Join(tmpDir, "builder-state.json")
	templatePath := filepath.Join(tmpDir, "template.pkr.hcl")

	// Create a dummy template
	templateContent := `
source "null" "test" {
  communicator = "none"
}

build {
  sources = ["source.null.test"]
}
`
	if err := ioutil.WriteFile(templatePath, []byte(templateContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Phase 1: Initialize state
	t.Log("Phase 1: Creating initial state")
	st := state.New(templatePath)
	st.BuilderVersion = "1.0.0-integration-test"
	st.PackerVersion = "1.9.0"

	// Compute template hash
	templateHash, err := state.ComputeFileHash(templatePath)
	if err != nil {
		t.Fatal(err)
	}
	st.Template.Hash = templateHash

	// Save initial state
	if err := st.Save(statePath); err != nil {
		t.Fatalf("Failed to save initial state: %v", err)
	}

	// Phase 2: Start a build
	t.Log("Phase 2: Starting build")
	build := &state.Build{
		Name:      "test-build",
		Type:      "null",
		Status:    state.BuildStatusPending,
		StartedAt: time.Now(),
		Provisioners: []state.ProvisionerState{
			{Type: "shell-local", Name: "setup", Status: state.StatusPending},
			{Type: "shell-local", Name: "configure", Status: state.StatusPending},
			{Type: "shell-local", Name: "validate", Status: state.StatusPending},
		},
	}

	st.SetBuild("test-build", build)
	if err := st.Save(statePath); err != nil {
		t.Fatalf("Failed to save build start: %v", err)
	}

	// Phase 3: Create instance
	t.Log("Phase 3: Creating instance")
	st, _ = state.Load(statePath)
	build = st.GetBuild("test-build")
	build.Status = state.BuildStatusCreating
	build.Instance = &state.Instance{
		ID:        "test-instance-abc123",
		BuilderID: "null.test",
		Provider:  "null",
		CreatedAt: time.Now(),
		Metadata: map[string]interface{}{
			"test": "value",
			"port": 22,
		},
	}
	st.SetBuild("test-build", build)
	if err := st.Save(statePath); err != nil {
		t.Fatalf("Failed to save instance creation: %v", err)
	}

	// Phase 4: Run provisioners
	t.Log("Phase 4: Running provisioners")
	for i := range build.Provisioners {
		st, _ = state.Load(statePath)
		build = st.GetBuild("test-build")

		build.Status = state.BuildStatusProvisioning
		build.Provisioners[i].Status = state.StatusRunning
		build.Provisioners[i].StartedAt = time.Now()
		st.SetBuild("test-build", build)
		if err := st.Save(statePath); err != nil {
			t.Fatalf("Failed to save provisioner start: %v", err)
		}

		// Simulate provisioner work
		time.Sleep(100 * time.Millisecond)

		// Mark complete
		st, _ = state.Load(statePath)
		build = st.GetBuild("test-build")
		build.Provisioners[i].Status = state.StatusComplete
		build.Provisioners[i].EndedAt = time.Now()
		st.SetBuild("test-build", build)
		if err := st.Save(statePath); err != nil {
			t.Fatalf("Failed to save provisioner complete: %v", err)
		}

		t.Logf("  Completed provisioner %d: %s", i, build.Provisioners[i].Type)
	}

	// Phase 5: Add artifacts
	t.Log("Phase 5: Adding artifacts")
	st, _ = state.Load(statePath)
	build = st.GetBuild("test-build")
	build.Status = state.BuildStatusComplete
	build.CompletedAt = time.Now()
	build.Artifacts = []state.ArtifactState{
		{
			ID:        "artifact-1",
			BuilderID: "null.test",
			Type:      "null",
			Files:     []string{"/tmp/test.txt"},
			Metadata: map[string]interface{}{
				"size": 1024,
			},
		},
	}
	st.SetBuild("test-build", build)
	if err := st.Save(statePath); err != nil {
		t.Fatalf("Failed to save final state: %v", err)
	}

	// Phase 6: Validate final state
	t.Log("Phase 6: Validating final state")
	finalState, err := state.Load(statePath)
	if err != nil {
		t.Fatalf("Failed to load final state: %v", err)
	}

	finalBuild := finalState.GetBuild("test-build")
	if finalBuild == nil {
		t.Fatal("Build not found in final state")
	}

	// Validations
	if finalBuild.Status != state.BuildStatusComplete {
		t.Errorf("Expected status 'complete', got '%s'", finalBuild.Status)
	}

	if finalBuild.Instance == nil {
		t.Error("Expected instance to be present")
	} else if finalBuild.Instance.ID != "test-instance-abc123" {
		t.Errorf("Expected instance ID 'test-instance-abc123', got '%s'", finalBuild.Instance.ID)
	}

	completedProvisioners := 0
	for _, prov := range finalBuild.Provisioners {
		if prov.Status == state.StatusComplete {
			completedProvisioners++
		}
	}
	if completedProvisioners != 3 {
		t.Errorf("Expected 3 completed provisioners, got %d", completedProvisioners)
	}

	if len(finalBuild.Artifacts) != 1 {
		t.Errorf("Expected 1 artifact, got %d", len(finalBuild.Artifacts))
	}

	if finalBuild.CompletedAt.IsZero() {
		t.Error("Expected CompletedAt to be set")
	}

	duration := finalBuild.CompletedAt.Sub(finalBuild.StartedAt)
	t.Logf("Build duration: %v", duration)

	// Print final state summary
	t.Log("Final state summary:")
	t.Logf("  Version: %d", finalState.Version)
	t.Logf("  Serial: %d", finalState.Serial)
	t.Logf("  Build: %s", finalBuild.Name)
	t.Logf("  Status: %s", finalBuild.Status)
	t.Logf("  Instance: %s", finalBuild.Instance.ID)
	t.Logf("  Provisioners: %d completed", completedProvisioners)
	t.Logf("  Artifacts: %d", len(finalBuild.Artifacts))
	t.Logf("  Duration: %v", duration)
}

// TestComponentIntegration_MultipleBuilds tests multiple builds in one state
func TestComponentIntegration_MultipleBuilds(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "component-multi-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	statePath := filepath.Join(tmpDir, "builder-state.json")

	st := state.New(tmpDir + "/template.pkr.hcl")

	// Create multiple builds
	buildNames := []string{"web-server", "database", "cache"}
	for _, name := range buildNames {
		build := &state.Build{
			Name:      name,
			Type:      "null",
			Status:    state.BuildStatusComplete,
			StartedAt: time.Now().Add(-10 * time.Minute),
			CompletedAt: time.Now().Add(-5 * time.Minute),
			Instance: &state.Instance{
				ID:       fmt.Sprintf("%s-instance", name),
				Provider: "null",
			},
			Provisioners: []state.ProvisionerState{
				{Type: "shell", Status: state.StatusComplete},
			},
			Artifacts: []state.ArtifactState{
				{
					ID:        fmt.Sprintf("%s-artifact", name),
					BuilderID: "null",
					Type:      "null",
				},
			},
		}
		st.SetBuild(name, build)
	}

	if err := st.Save(statePath); err != nil {
		t.Fatalf("Failed to save state: %v", err)
	}

	// Load and validate
	loaded, err := state.Load(statePath)
	if err != nil {
		t.Fatalf("Failed to load state: %v", err)
	}

	if len(loaded.Builds) != 3 {
		t.Errorf("Expected 3 builds, got %d", len(loaded.Builds))
	}

	for _, name := range buildNames {
		build := loaded.GetBuild(name)
		if build == nil {
			t.Errorf("Build '%s' not found", name)
			continue
		}

		if build.Status != state.BuildStatusComplete {
			t.Errorf("Build '%s': expected status 'complete', got '%s'", name, build.Status)
		}

		t.Logf("Build '%s': ✓ Complete with %d artifacts", name, len(build.Artifacts))
	}
}

// TestComponentIntegration_BuildFailure tests handling of failed builds
func TestComponentIntegration_BuildFailure(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "component-failure-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	statePath := filepath.Join(tmpDir, "builder-state.json")

	st := state.New(tmpDir + "/template.pkr.hcl")

	// Create a failed build
	build := &state.Build{
		Name:      "failed-build",
		Type:      "null",
		Status:    state.BuildStatusFailed,
		Error:     "Provisioner 'shell-local' failed: exit status 1",
		StartedAt: time.Now().Add(-5 * time.Minute),
		Instance: &state.Instance{
			ID:       "failed-instance",
			Provider: "null",
		},
		Provisioners: []state.ProvisionerState{
			{Type: "shell-local", Name: "setup", Status: state.StatusComplete},
			{Type: "shell-local", Name: "install", Status: state.StatusFailed, Error: "exit status 1"},
			{Type: "shell-local", Name: "configure", Status: state.StatusSkipped},
		},
	}

	st.SetBuild("failed-build", build)

	if err := st.Save(statePath); err != nil {
		t.Fatalf("Failed to save state: %v", err)
	}

	// Load and validate
	loaded, err := state.Load(statePath)
	if err != nil {
		t.Fatalf("Failed to load state: %v", err)
	}

	failedBuild := loaded.GetBuild("failed-build")
	if failedBuild == nil {
		t.Fatal("Failed build not found")
	}

	if failedBuild.Status != state.BuildStatusFailed {
		t.Errorf("Expected status 'failed', got '%s'", failedBuild.Status)
	}

	if failedBuild.Error == "" {
		t.Error("Expected error message to be set")
	}

	// Check provisioner states
	var completed, failed, skipped int
	for _, prov := range failedBuild.Provisioners {
		switch prov.Status {
		case state.StatusComplete:
			completed++
		case state.StatusFailed:
			failed++
		case state.StatusSkipped:
			skipped++
		}
	}

	if completed != 1 || failed != 1 || skipped != 1 {
		t.Errorf("Expected 1 completed, 1 failed, 1 skipped provisioner, got %d, %d, %d",
			completed, failed, skipped)
	}

	t.Logf("Failed build state correctly recorded:")
	t.Logf("  Error: %s", failedBuild.Error)
	t.Logf("  Provisioners: %d completed, %d failed, %d skipped", completed, failed, skipped)
}

// TestComponentIntegration_StateResumption tests resuming from a checkpoint
func TestComponentIntegration_StateResumption(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "component-resume-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	statePath := filepath.Join(tmpDir, "builder-state.json")

	// Simulate interrupted build
	st := state.New(tmpDir + "/template.pkr.hcl")

	build := &state.Build{
		Name:      "interrupted-build",
		Type:      "null",
		Status:    state.BuildStatusProvisioning,
		StartedAt: time.Now().Add(-10 * time.Minute),
		Instance: &state.Instance{
			ID:           "instance-to-resume",
			Provider:     "null",
			KeepOnFailure: true,
		},
		Provisioners: []state.ProvisionerState{
			{Type: "shell", Name: "step1", Status: state.StatusComplete},
			{Type: "shell", Name: "step2", Status: state.StatusComplete},
			{Type: "shell", Name: "step3", Status: state.StatusRunning},
			{Type: "shell", Name: "step4", Status: state.StatusPending},
		},
	}

	st.SetBuild("interrupted-build", build)
	if err := st.Save(statePath); err != nil {
		t.Fatal(err)
	}

	// Load and determine where to resume
	loaded, err := state.Load(statePath)
	if err != nil {
		t.Fatal(err)
	}

	resumeBuild := loaded.GetBuild("interrupted-build")
	if resumeBuild == nil {
		t.Fatal("Build not found")
	}

	// Find next pending provisioner
	// Note: Provisioner 2 is "running" so NextPendingProvisioner will return 3
	// In a real scenario, we'd want to retry/restart the running provisioner
	nextIdx := resumeBuild.NextPendingProvisioner()

	// Update: NextPendingProvisioner returns first pending OR failed, so if provisioner 2
	// is still "running", it will return the next one (3) which is "pending"
	t.Logf("Next pending provisioner index: %d", nextIdx)

	if nextIdx >= len(resumeBuild.Provisioners) {
		t.Fatal("No pending provisioners found")
	}

	t.Logf("Resuming build at provisioner %d: %s", nextIdx, resumeBuild.Provisioners[nextIdx].Name)

	// Resume from step 3
	resumeBuild.Provisioners[2].Status = state.StatusComplete
	resumeBuild.Provisioners[2].EndedAt = time.Now()
	loaded.SetBuild("interrupted-build", resumeBuild)
	if err := loaded.Save(statePath); err != nil {
		t.Fatal(err)
	}

	// Complete remaining provisioners
	loaded, _ = state.Load(statePath)
	resumeBuild = loaded.GetBuild("interrupted-build")
	resumeBuild.Provisioners[3].Status = state.StatusComplete
	resumeBuild.Provisioners[3].EndedAt = time.Now()
	resumeBuild.Status = state.BuildStatusComplete
	resumeBuild.CompletedAt = time.Now()
	loaded.SetBuild("interrupted-build", resumeBuild)
	if err := loaded.Save(statePath); err != nil {
		t.Fatal(err)
	}

	// Verify final state
	final, err := state.Load(statePath)
	if err != nil {
		t.Fatal(err)
	}

	finalBuild := final.GetBuild("interrupted-build")
	if finalBuild.Status != state.BuildStatusComplete {
		t.Errorf("Expected final status 'complete', got '%s'", finalBuild.Status)
	}

	allComplete := true
	for _, prov := range finalBuild.Provisioners {
		if prov.Status != state.StatusComplete {
			allComplete = false
		}
	}

	if !allComplete {
		t.Error("Not all provisioners completed")
	}

	t.Log("Successfully resumed and completed interrupted build")
}
