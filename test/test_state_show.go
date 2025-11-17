package main

import (
	"fmt"
	"os"

	"github.com/hashicorp/packer/builder/state"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: test_state_show <state-file-path>")
		os.Exit(1)
	}

	statePath := os.Args[1]

	// Load state
	st, err := state.Load(statePath)
	if err != nil {
		fmt.Printf("Error loading state: %s\n", err)
		os.Exit(1)
	}

	if st == nil {
		fmt.Println("No state file found.")
		os.Exit(0)
	}

	// Pretty print the state
	fmt.Printf("State file: %s\n", statePath)
	fmt.Printf("Version: %d (serial: %d)\n", st.Version, st.Serial)
	fmt.Printf("Template: %s\n", st.Template.Path)
	fmt.Printf("Template Hash: %s\n", st.Template.Hash)
	fmt.Println()

	if len(st.Builds) == 0 {
		fmt.Println("No builds in state.")
		return
	}

	fmt.Printf("Builds (%d):\n", len(st.Builds))
	for name, build := range st.Builds {
		fmt.Printf("\n  %s:\n", name)
		fmt.Printf("    Type: %s\n", build.Type)
		fmt.Printf("    Status: %s\n", build.Status)

		if build.Instance != nil {
			fmt.Printf("    Instance:\n")
			fmt.Printf("      ID: %s\n", build.Instance.ID)
			if build.Instance.PublicIP != "" {
				fmt.Printf("      IP: %s\n", build.Instance.PublicIP)
			}
			if build.Instance.Provider != "" {
				fmt.Printf("      Provider: %s\n", build.Instance.Provider)
			}
		}

		if len(build.Provisioners) > 0 {
			completedCount := 0
			for _, p := range build.Provisioners {
				if p.Status == state.StatusComplete {
					completedCount++
				}
			}
			fmt.Printf("    Provisioners: %d/%d complete\n", completedCount, len(build.Provisioners))

			// Show details of each provisioner
			for i, prov := range build.Provisioners {
				status := ""
				switch prov.Status {
				case state.StatusComplete:
					status = "✓"
				case state.StatusFailed:
					status = "✗"
				case state.StatusPending:
					status = "○"
				default:
					status = "?"
				}
				fmt.Printf("      [%s] %d. %s\n", status, i+1, prov.Type)
				if prov.Error != "" {
					fmt.Printf("          Error: %s\n", prov.Error)
				}
			}
		}

		if len(build.Artifacts) > 0 {
			fmt.Printf("    Artifacts:\n")
			for _, art := range build.Artifacts {
				fmt.Printf("      - %s (%s)\n", art.ID, art.BuilderID)
			}
		}

		if !build.CompletedAt.IsZero() {
			fmt.Printf("    Completed: %s\n", build.CompletedAt.Format("2006-01-02 15:04:05"))
		}
	}
}
