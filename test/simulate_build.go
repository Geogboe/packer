package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/hashicorp/packer/builder/state"
)

// MockBuildSimulator simulates a build with controllable failures
type MockBuildSimulator struct {
	BuildName        string
	TemplatePath     string
	StatePath        string
	FailAtProvisioner int // -1 = no fail, 0+ = fail at index
	ProvisionerCount int
}

func (m *MockBuildSimulator) Run() error {
	fmt.Printf("=== Mock Build Simulator ===\n")
	fmt.Printf("Build: %s\n", m.BuildName)
	fmt.Printf("State: %s\n\n", m.StatePath)

	// Load or create state
	manager := state.NewManager(m.StatePath)
	st, err := manager.Load()
	if err != nil {
		return fmt.Errorf("failed to load state: %w", err)
	}
	defer manager.Unlock()

	if st == nil {
		st = state.New(m.TemplatePath)
		fmt.Println("Created new state")
	} else {
		fmt.Printf("Loaded existing state (serial: %d)\n", st.Serial)
	}

	// Check if build exists in state
	build := st.GetBuild(m.BuildName)

	if build != nil && build.IsComplete() {
		fmt.Println("\n✓ Build already complete in state")
		fmt.Printf("  Completed at: %s\n", build.CompletedAt.Format(time.RFC3339))
		fmt.Printf("  Artifacts: %d\n", len(build.Artifacts))
		return nil
	}

	// Initialize build if needed
	if build == nil {
		fmt.Println("\nInitializing new build...")
		build = &state.Build{
			Name:         m.BuildName,
			Type:         "mock-builder",
			Status:       state.BuildStatusPending,
			StartedAt:    time.Now(),
			Provisioners: make([]state.ProvisionerState, m.ProvisionerCount),
		}

		for i := 0; i < m.ProvisionerCount; i++ {
			build.Provisioners[i] = state.ProvisionerState{
				Type:   fmt.Sprintf("shell-%d", i+1),
				Status: state.StatusPending,
			}
		}

		st.SetBuild(m.BuildName, build)
	}

	// Simulate builder phase
	if build.Instance == nil {
		fmt.Println("\n==> Creating VM instance...")
		time.Sleep(500 * time.Millisecond)

		build.Instance = &state.Instance{
			ID:        fmt.Sprintf("i-%d", time.Now().Unix()),
			Provider:  "mock",
			Region:    "mock-region-1",
			PublicIP:  "192.168.1.100",
			SSHUser:   "ubuntu",
			SSHPort:   22,
			CreatedAt: time.Now(),
			KeepOnFailure: true,
		}

		build.Status = state.BuildStatusProvisioning
		fmt.Printf("✓ Instance created: %s\n", build.Instance.ID)
		fmt.Printf("  IP: %s\n", build.Instance.PublicIP)

		// Save checkpoint
		if err := manager.Save(); err != nil {
			return fmt.Errorf("failed to save after instance creation: %w", err)
		}
		fmt.Println("  [CHECKPOINT: Instance created]")
	} else {
		fmt.Printf("\n==> Found existing instance: %s\n", build.Instance.ID)
		fmt.Printf("  IP: %s\n", build.Instance.PublicIP)
		fmt.Println("  Resuming from checkpoint...")
	}

	// Simulate provisioners
	fmt.Println("\n==> Running provisioners...")
	for i := 0; i < m.ProvisionerCount; i++ {
		if build.Provisioners[i].Status == state.StatusComplete {
			fmt.Printf("  ✓ Provisioner %d (%s): already complete (skipped)\n",
				i+1, build.Provisioners[i].Type)
			continue
		}

		fmt.Printf("  → Provisioner %d (%s): running...\n",
			i+1, build.Provisioners[i].Type)

		// Clear error if retrying a failed provisioner
		build.Provisioners[i].Error = ""
		build.Provisioners[i].Status = state.StatusRunning
		build.Provisioners[i].StartedAt = time.Now()

		// Simulate work
		time.Sleep(300 * time.Millisecond)

		// Check for injected failure
		if m.FailAtProvisioner == i {
			build.Provisioners[i].Status = state.StatusFailed
			build.Provisioners[i].Error = "Simulated failure!"
			build.Status = state.BuildStatusFailed
			build.Error = fmt.Sprintf("Provisioner %d failed", i+1)

			if err := manager.Save(); err != nil {
				return fmt.Errorf("failed to save after failure: %w", err)
			}

			fmt.Printf("  ✗ Provisioner %d FAILED: %s\n", i+1, build.Provisioners[i].Error)
			fmt.Println("\n  [CHECKPOINT: Build failed, state saved]")
			fmt.Println("  Instance kept alive for debugging")
			return fmt.Errorf("build failed at provisioner %d", i+1)
		}

		// Success
		build.Provisioners[i].Status = state.StatusComplete
		build.Provisioners[i].EndedAt = time.Now()
		fmt.Printf("  ✓ Provisioner %d: complete\n", i+1)

		// Save checkpoint after each provisioner
		if err := manager.Save(); err != nil {
			return fmt.Errorf("failed to save after provisioner %d: %w", i+1, err)
		}
		fmt.Printf("    [CHECKPOINT: Provisioner %d saved]\n", i+1)
	}

	// All provisioners complete - create artifact
	fmt.Println("\n==> Creating artifacts...")
	time.Sleep(300 * time.Millisecond)

	build.Status = state.BuildStatusComplete
	build.CompletedAt = time.Now()
	build.Artifacts = []state.ArtifactState{
		{
			ID:        fmt.Sprintf("ami-%d", time.Now().Unix()),
			BuilderID: "mock-builder",
			Type:      "ami",
		},
	}

	fmt.Printf("✓ Artifact created: %s\n", build.Artifacts[0].ID)

	// Cleanup instance
	fmt.Println("\n==> Destroying instance...")
	time.Sleep(200 * time.Millisecond)
	fmt.Printf("✓ Instance %s destroyed\n", build.Instance.ID)

	// Final save
	if err := manager.Save(); err != nil {
		return fmt.Errorf("failed to save final state: %w", err)
	}

	fmt.Println("\n[CHECKPOINT: Build complete]")
	fmt.Println("\n✅ Build finished successfully!")
	return nil
}

func main() {
	buildName := flag.String("build", "mock.example", "Build name")
	template := flag.String("template", "mock.pkr.hcl", "Template path")
	stateDir := flag.String("state-dir", ".", "State directory")
	failAt := flag.Int("fail-at", -1, "Fail at provisioner index (-1 = no fail)")
	provisioners := flag.Int("provisioners", 3, "Number of provisioners")

	flag.Parse()

	statePath := filepath.Join(*stateDir, ".packer.d", "builder-state.json")

	simulator := &MockBuildSimulator{
		BuildName:         *buildName,
		TemplatePath:      *template,
		StatePath:         statePath,
		FailAtProvisioner: *failAt,
		ProvisionerCount:  *provisioners,
	}

	if err := simulator.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "\n❌ Error: %s\n", err)
		os.Exit(1)
	}
}
