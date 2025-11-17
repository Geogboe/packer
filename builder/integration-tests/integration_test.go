package integration_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/packer/builder/state"
)

// TestBasicNullBuild tests a basic null builder build
func TestBasicNullBuild(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	tmpDir, err := ioutil.TempDir("", "integration-basic-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	templatePath := "templates/basic-null.pkr.hcl"

	// Run packer build
	output, err := runPackerBuild(t, tmpDir, templatePath)
	if err != nil {
		t.Fatalf("Build failed: %v\nOutput:\n%s", err, output)
	}

	t.Logf("Build output:\n%s", output)

	// Validate build succeeded
	if !strings.Contains(output, "Build 'basic-null-build' finished") {
		t.Error("Expected build to finish successfully")
	}

	// Check for state file
	statePath := filepath.Join(tmpDir, ".packer.d", "builder-state.json")
	if _, err := os.Stat(statePath); os.IsNotExist(err) {
		t.Log("State file not created (expected if using regular packer)")
	} else {
		validateStateFile(t, statePath)
	}
}

// TestFileBuilder tests the file builder
func TestFileBuilder(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	tmpDir, err := ioutil.TempDir("", "integration-file-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create output directory
	outputDir := filepath.Join(tmpDir, "output")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Copy template to tmpdir and update paths
	templatePath := filepath.Join(tmpDir, "file-builder.pkr.hcl")
	templateContent, err := ioutil.ReadFile("templates/file-builder.pkr.hcl")
	if err != nil {
		t.Fatal(err)
	}

	// Update output path to tmpDir
	updatedContent := strings.ReplaceAll(string(templateContent),
		"output/test-file.txt",
		filepath.Join(outputDir, "test-file.txt"))

	if err := ioutil.WriteFile(templatePath, []byte(updatedContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Run build
	output, err := runPackerBuild(t, tmpDir, templatePath)
	if err != nil {
		t.Fatalf("Build failed: %v\nOutput:\n%s", err, output)
	}

	t.Logf("Build output:\n%s", output)

	// Validate file was created
	testFilePath := filepath.Join(outputDir, "test-file.txt")
	if _, err := os.Stat(testFilePath); os.IsNotExist(err) {
		t.Errorf("Expected test file to be created at %s", testFilePath)
	} else {
		content, _ := ioutil.ReadFile(testFilePath)
		t.Logf("Created file content:\n%s", content)

		if !strings.Contains(string(content), "Hello from Packer Fork Builder!") {
			t.Error("File content doesn't match expected")
		}
	}
}

// TestMultiProvisioner tests multiple provisioners in sequence
func TestMultiProvisioner(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	tmpDir, err := ioutil.TempDir("", "integration-multi-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	templatePath := "templates/multi-provisioner.pkr.hcl"

	start := time.Now()
	output, err := runPackerBuild(t, tmpDir, templatePath)
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("Build failed: %v\nOutput:\n%s", err, output)
	}

	t.Logf("Build completed in %v", duration)
	t.Logf("Build output:\n%s", output)

	// Validate all provisioners ran
	provisionerStages := []string{
		"Provisioner 1: Setup",
		"Provisioner 2: Processing",
		"Provisioner 3: Validation",
		"Provisioner 4: Cleanup",
	}

	for _, stage := range provisionerStages {
		if !strings.Contains(output, stage) {
			t.Errorf("Expected provisioner stage '%s' in output", stage)
		}
	}

	// Build should take at least 4 seconds (4 provisioners with 1s sleep each)
	if duration < 4*time.Second {
		t.Logf("Warning: Build completed faster than expected (%v)", duration)
	}
}

// TestVariableInterpolation tests variable substitution
func TestVariableInterpolation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	tmpDir, err := ioutil.TempDir("", "integration-vars-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	templatePath := "templates/variables.pkr.hcl"

	// Test with default variable
	t.Run("default_variable", func(t *testing.T) {
		output, err := runPackerBuild(t, tmpDir, templatePath)
		if err != nil {
			t.Fatalf("Build failed: %v\nOutput:\n%s", err, output)
		}

		if !strings.Contains(output, "TEST_VAR=default-value") {
			t.Error("Expected default variable value in output")
		}
	})

	// Test with custom variable
	t.Run("custom_variable", func(t *testing.T) {
		output, err := runPackerBuildWithVars(t, tmpDir, templatePath,
			map[string]string{"test_value": "custom-test-value"})
		if err != nil {
			t.Fatalf("Build failed: %v\nOutput:\n%s", err, output)
		}

		if !strings.Contains(output, "TEST_VAR=custom-test-value") {
			t.Error("Expected custom variable value in output")
		}
	})
}

// TestStateManagement tests state file creation and management
func TestStateManagement(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// This test would require the custom builder binary
	// For now, we'll test state management directly

	tmpDir, err := ioutil.TempDir("", "integration-state-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	statePath := filepath.Join(tmpDir, "test-state.json")

	// Create and save state
	st := state.New(filepath.Join(tmpDir, "template.pkr.hcl"))
	st.BuilderVersion = "1.0.0-test"
	st.PackerVersion = "1.9.0"

	build := &state.Build{
		Name:      "test-build",
		Type:      "null",
		Status:    state.BuildStatusComplete,
		StartedAt: time.Now(),
		Instance: &state.Instance{
			ID:       "test-instance-123",
			Provider: "null",
		},
		Provisioners: []state.ProvisionerState{
			{Type: "shell-local", Status: state.StatusComplete},
		},
		Artifacts: []state.ArtifactState{
			{
				ID:        "artifact-1",
				BuilderID: "null",
				Type:      "manifest",
			},
		},
	}

	st.SetBuild("test-build", build)

	if err := st.Save(statePath); err != nil {
		t.Fatalf("Failed to save state: %v", err)
	}

	// Load and validate state
	loaded, err := state.Load(statePath)
	if err != nil {
		t.Fatalf("Failed to load state: %v", err)
	}

	if loaded.BuilderVersion != "1.0.0-test" {
		t.Errorf("Expected BuilderVersion '1.0.0-test', got '%s'", loaded.BuilderVersion)
	}

	loadedBuild := loaded.GetBuild("test-build")
	if loadedBuild == nil {
		t.Fatal("Expected build to be loaded")
	}

	if loadedBuild.Status != state.BuildStatusComplete {
		t.Errorf("Expected status 'complete', got '%s'", loadedBuild.Status)
	}

	if len(loadedBuild.Provisioners) != 1 {
		t.Errorf("Expected 1 provisioner, got %d", len(loadedBuild.Provisioners))
	}

	t.Log("State management test passed")
}

// TestConcurrentBuilds tests running multiple builds concurrently
func TestConcurrentBuilds(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	tmpDir, err := ioutil.TempDir("", "integration-concurrent-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	numBuilds := 3
	results := make(chan error, numBuilds)

	// Run multiple builds concurrently
	for i := 0; i < numBuilds; i++ {
		go func(buildNum int) {
			buildDir := filepath.Join(tmpDir, fmt.Sprintf("build-%d", buildNum))
			if err := os.MkdirAll(buildDir, 0755); err != nil {
				results <- err
				return
			}

			output, err := runPackerBuild(t, buildDir, "../templates/basic-null.pkr.hcl")
			if err != nil {
				results <- fmt.Errorf("build %d failed: %v\nOutput:\n%s", buildNum, err, output)
				return
			}

			t.Logf("Build %d completed successfully", buildNum)
			results <- nil
		}(i)
	}

	// Wait for all builds
	var errors []error
	for i := 0; i < numBuilds; i++ {
		if err := <-results; err != nil {
			errors = append(errors, err)
		}
	}

	if len(errors) > 0 {
		t.Errorf("Some builds failed:")
		for _, err := range errors {
			t.Errorf("  - %v", err)
		}
	}
}

// Helper functions

func runPackerBuild(t *testing.T, workdir string, templatePath string) (string, error) {
	return runPackerBuildWithVars(t, workdir, templatePath, nil)
}

func runPackerBuildWithVars(t *testing.T, workdir string, templatePath string, vars map[string]string) (string, error) {
	// Try to find packer binary
	packerBin, err := findPackerBinary()
	if err != nil {
		t.Skip("Packer binary not found, skipping integration test")
		return "", err
	}

	args := []string{"build"}

	// Add variables
	for key, value := range vars {
		args = append(args, "-var", fmt.Sprintf("%s=%s", key, value))
	}

	args = append(args, templatePath)

	cmd := exec.Command(packerBin, args...)
	cmd.Dir = workdir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	t.Logf("Running: %s %v", packerBin, args)
	t.Logf("Working directory: %s", workdir)

	err = cmd.Run()

	output := stdout.String() + stderr.String()

	return output, err
}

func findPackerBinary() (string, error) {
	// Check common locations
	locations := []string{
		"/tmp/packer",
		"../bin/packer",
		"../../bin/packer",
		"/usr/local/bin/packer",
		"/usr/bin/packer",
	}

	for _, loc := range locations {
		if _, err := os.Stat(loc); err == nil {
			return loc, nil
		}
	}

	// Try PATH
	path, err := exec.LookPath("packer")
	if err == nil {
		return path, nil
	}

	return "", fmt.Errorf("packer binary not found")
}

func validateStateFile(t *testing.T, statePath string) {
	data, err := ioutil.ReadFile(statePath)
	if err != nil {
		t.Errorf("Failed to read state file: %v", err)
		return
	}

	var st state.State
	if err := json.Unmarshal(data, &st); err != nil {
		t.Errorf("Failed to parse state file: %v", err)
		return
	}

	t.Logf("State file contents:")
	t.Logf("  Version: %d", st.Version)
	t.Logf("  Serial: %d", st.Serial)
	t.Logf("  Lineage: %s", st.Lineage)
	t.Logf("  Builds: %d", len(st.Builds))

	for name, build := range st.Builds {
		t.Logf("  Build '%s':", name)
		t.Logf("    Type: %s", build.Type)
		t.Logf("    Status: %s", build.Status)
		t.Logf("    Provisioners: %d", len(build.Provisioners))
		t.Logf("    Artifacts: %d", len(build.Artifacts))
	}
}
