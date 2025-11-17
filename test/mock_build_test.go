package test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hashicorp/packer/builder/state"
)

func TestStateBasicOperations(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "test-state.json")

	t.Run("Create new state", func(t *testing.T) {
		st := state.New("template.pkr.hcl")
		if st.Version != 1 {
			t.Errorf("Expected version 1, got %d", st.Version)
		}
		if st.Lineage == "" {
			t.Error("Expected lineage to be set")
		}
	})

	t.Run("Save and load state", func(t *testing.T) {
		// Create and save
		st := state.New("template.pkr.hcl")
		st.Template.Hash = "sha256:test123"
		st.Template.Variables = map[string]string{"foo": "bar"}

		if err := st.Save(statePath); err != nil {
			t.Fatalf("Failed to save state: %s", err)
		}

		// Load
		loaded, err := state.Load(statePath)
		if err != nil {
			t.Fatalf("Failed to load state: %s", err)
		}

		if loaded.Template.Hash != "sha256:test123" {
			t.Errorf("Hash mismatch: got %s", loaded.Template.Hash)
		}

		if loaded.Template.Variables["foo"] != "bar" {
			t.Error("Variables not preserved")
		}
	})

	t.Run("State locking", func(t *testing.T) {
		st := state.New("template.pkr.hcl")
		st.Save(statePath)

		manager1 := state.NewManager(statePath)
		_, err := manager1.Load()
		if err != nil {
			t.Fatalf("Manager1 failed to load: %s", err)
		}

		// Try to lock again - should fail
		manager2 := state.NewManager(statePath)
		_, err = manager2.Load()
		if err == nil {
			t.Error("Expected lock error, got nil")
		}

		// Unlock and try again
		manager1.Unlock()

		_, err = manager2.Load()
		if err != nil {
			t.Errorf("Should succeed after unlock: %s", err)
		}
		manager2.Unlock()
	})
}

func TestBuildStateManagement(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "build-state.json")

	t.Run("Add build to state", func(t *testing.T) {
		st := state.New("template.pkr.hcl")

		build := &state.Build{
			Name:      "amazon-ebs.ubuntu",
			Type:      "amazon-ebs",
			Status:    state.BuildStatusPending,
			StartedAt: time.Now(),
		}

		st.SetBuild("amazon-ebs.ubuntu", build)

		if len(st.Builds) != 1 {
			t.Errorf("Expected 1 build, got %d", len(st.Builds))
		}

		retrieved := st.GetBuild("amazon-ebs.ubuntu")
		if retrieved == nil {
			t.Fatal("Failed to retrieve build")
		}

		if retrieved.Name != "amazon-ebs.ubuntu" {
			t.Errorf("Name mismatch: %s", retrieved.Name)
		}
	})

	t.Run("Build with instance", func(t *testing.T) {
		st := state.New("template.pkr.hcl")

		build := &state.Build{
			Name:   "test-build",
			Type:   "mock",
			Status: state.BuildStatusProvisioning,
			Instance: &state.Instance{
				ID:         "i-1234567890",
				Provider:   "aws",
				Region:     "us-east-1",
				PublicIP:   "54.123.45.67",
				SSHUser:    "ubuntu",
				SSHPort:    22,
				CreatedAt:  time.Now(),
			},
			Provisioners: []state.ProvisionerState{
				{Type: "shell", Status: state.StatusComplete},
				{Type: "file", Status: state.StatusPending},
			},
		}

		st.SetBuild("test-build", build)

		if err := st.Save(statePath); err != nil {
			t.Fatalf("Failed to save: %s", err)
		}

		// Load and verify
		loaded, err := state.Load(statePath)
		if err != nil {
			t.Fatalf("Failed to load: %s", err)
		}

		loadedBuild := loaded.GetBuild("test-build")
		if loadedBuild == nil {
			t.Fatal("Build not found")
		}

		if !loadedBuild.HasInstance() {
			t.Error("Instance not preserved")
		}

		if loadedBuild.Instance.ID != "i-1234567890" {
			t.Errorf("Instance ID mismatch: %s", loadedBuild.Instance.ID)
		}

		if len(loadedBuild.Provisioners) != 2 {
			t.Errorf("Expected 2 provisioners, got %d", len(loadedBuild.Provisioners))
		}

		if !loadedBuild.ProvisionerComplete(0) {
			t.Error("First provisioner should be complete")
		}

		if loadedBuild.ProvisionerComplete(1) {
			t.Error("Second provisioner should not be complete")
		}
	})

	t.Run("Build completion", func(t *testing.T) {
		st := state.New("template.pkr.hcl")

		build := &state.Build{
			Name:        "test-build",
			Type:        "mock",
			Status:      state.BuildStatusComplete,
			CompletedAt: time.Now(),
			Artifacts: []state.ArtifactState{
				{
					ID:        "ami-abc123",
					BuilderID: "amazon-ebs",
					Type:      "ami",
				},
			},
		}

		st.SetBuild("test-build", build)

		if !build.IsComplete() {
			t.Error("Build should be complete")
		}

		if len(build.Artifacts) != 1 {
			t.Error("Artifact not preserved")
		}
	})
}

func TestProvisionerTracking(t *testing.T) {
	build := &state.Build{
		Name: "test",
		Provisioners: []state.ProvisionerState{
			{Type: "shell-1", Status: state.StatusComplete},
			{Type: "shell-2", Status: state.StatusComplete},
			{Type: "shell-3", Status: state.StatusFailed},
			{Type: "shell-4", Status: state.StatusPending},
		},
	}

	t.Run("Next pending provisioner", func(t *testing.T) {
		next := build.NextPendingProvisioner()
		if next != 2 {
			t.Errorf("Expected next pending at index 2, got %d", next)
		}
	})

	t.Run("Provisioner completion check", func(t *testing.T) {
		if !build.ProvisionerComplete(0) {
			t.Error("Provisioner 0 should be complete")
		}

		if build.ProvisionerComplete(2) {
			t.Error("Provisioner 2 should not be complete (failed)")
		}

		if build.ProvisionerComplete(3) {
			t.Error("Provisioner 3 should not be complete (pending)")
		}
	})
}

func TestFingerprinting(t *testing.T) {
	t.Run("Compute file hash", func(t *testing.T) {
		tmpDir := t.TempDir()
		testFile := filepath.Join(tmpDir, "test.txt")

		if err := os.WriteFile(testFile, []byte("hello world"), 0644); err != nil {
			t.Fatal(err)
		}

		hash1, err := state.ComputeFileHash(testFile)
		if err != nil {
			t.Fatalf("Failed to compute hash: %s", err)
		}

		if hash1 == "" {
			t.Error("Hash should not be empty")
		}

		// Same content = same hash
		hash2, err := state.ComputeFileHash(testFile)
		if err != nil {
			t.Fatal(err)
		}

		if hash1 != hash2 {
			t.Error("Hashes should match for same content")
		}

		// Different content = different hash
		os.WriteFile(testFile, []byte("goodbye world"), 0644)
		hash3, err := state.ComputeFileHash(testFile)
		if err != nil {
			t.Fatal(err)
		}

		if hash1 == hash3 {
			t.Error("Hashes should differ for different content")
		}
	})

	t.Run("Compute state fingerprint", func(t *testing.T) {
		st := state.New("template.pkr.hcl")
		st.Template.Hash = "sha256:abc123"
		st.Template.Variables = map[string]string{
			"var1": "value1",
			"var2": "value2",
		}

		fp1 := st.ComputeFingerprint()

		// Same state = same fingerprint
		fp2 := st.ComputeFingerprint()
		if fp1 != fp2 {
			t.Error("Fingerprints should match")
		}

		// Change variable = different fingerprint
		st.Template.Variables["var1"] = "changed"
		fp3 := st.ComputeFingerprint()
		if fp1 == fp3 {
			t.Error("Fingerprints should differ after change")
		}
	})
}

func TestInputChangeDetection(t *testing.T) {
	tmpDir := t.TempDir()
	statePath := filepath.Join(tmpDir, "change-state.json")

	manager := state.NewManager(statePath)
	st, _ := manager.Load()
	defer manager.Unlock()

	// Initial state
	st.Template.Hash = "sha256:original"
	st.Template.Variables = map[string]string{"foo": "bar"}
	manager.Save()

	t.Run("No changes", func(t *testing.T) {
		changed := manager.InputsChanged(
			"sha256:original",
			map[string]string{"foo": "bar"},
			map[string]string{},
		)

		if changed {
			t.Error("Should not detect changes")
		}
	})

	t.Run("Template changed", func(t *testing.T) {
		changed := manager.InputsChanged(
			"sha256:modified",
			map[string]string{"foo": "bar"},
			map[string]string{},
		)

		if !changed {
			t.Error("Should detect template change")
		}
	})

	t.Run("Variables changed", func(t *testing.T) {
		changed := manager.InputsChanged(
			"sha256:original",
			map[string]string{"foo": "changed"},
			map[string]string{},
		)

		if !changed {
			t.Error("Should detect variable change")
		}
	})

	t.Run("New variable added", func(t *testing.T) {
		changed := manager.InputsChanged(
			"sha256:original",
			map[string]string{"foo": "bar", "new": "var"},
			map[string]string{},
		)

		if !changed {
			t.Error("Should detect new variable")
		}
	})
}

// Run all tests and report
func TestMain(m *testing.M) {
	fmt.Println("=== Running Builder State Management Tests ===")
	fmt.Println()
	code := m.Run()
	if code == 0 {
		fmt.Println()
		fmt.Println("✅ All tests passed!")
	} else {
		fmt.Println()
		fmt.Println("❌ Some tests failed")
	}
	os.Exit(code)
}
