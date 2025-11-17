package state

import (
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestStateStress_ConcurrentReadsWrites tests concurrent access patterns
func TestStateStress_ConcurrentReadsWrites(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "state-stress-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	statePath := filepath.Join(tmpDir, "test.json")
	state := New("/tmp/template.pkr.hcl")

	// Save initial state
	if err := state.Save(statePath); err != nil {
		t.Fatal(err)
	}

	const (
		numReaders = 50
		numWriters = 20
		duration   = 5 * time.Second
	)

	var wg sync.WaitGroup
	stop := make(chan struct{})
	errors := make(chan error, numReaders+numWriters)

	// Start readers
	for i := 0; i < numReaders; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					_, err := Load(statePath)
					if err != nil {
						errors <- fmt.Errorf("reader %d: %w", id, err)
						return
					}
					time.Sleep(time.Millisecond * time.Duration(rand.Intn(10)))
				}
			}
		}(i)
	}

	// Start writers
	for i := 0; i < numWriters; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					s, err := Load(statePath)
					if err != nil {
						errors <- fmt.Errorf("writer %d load: %w", id, err)
						return
					}
					if s == nil {
						s = New("/tmp/template.pkr.hcl")
					}

					// Add/modify builds
					buildName := fmt.Sprintf("build-%d-%d", id, rand.Intn(10))
					s.SetBuild(buildName, &Build{
						Name:   buildName,
						Type:   "amazon-ebs",
						Status: BuildStatusComplete,
					})

					if err := s.Save(statePath); err != nil {
						errors <- fmt.Errorf("writer %d save: %w", id, err)
						return
					}
					time.Sleep(time.Millisecond * time.Duration(rand.Intn(20)))
				}
			}
		}(i)
	}

	// Run for duration
	time.Sleep(duration)
	close(stop)
	wg.Wait()
	close(errors)

	// Check for errors
	errorCount := 0
	for err := range errors {
		t.Logf("Error: %v", err)
		errorCount++
	}

	if errorCount > 0 {
		t.Fatalf("Encountered %d errors during concurrent stress test", errorCount)
	}

	// Verify final state is valid JSON
	finalState, err := Load(statePath)
	if err != nil {
		t.Fatalf("Final state is corrupted: %v", err)
	}
	if finalState == nil {
		t.Fatal("Final state is nil")
	}

	t.Logf("Final state has %d builds after %v of concurrent operations", len(finalState.Builds), duration)
}

// TestStateStress_LargeState tests handling of very large state files
func TestStateStress_LargeState(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "state-large-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	statePath := filepath.Join(tmpDir, "large.json")
	state := New("/tmp/template.pkr.hcl")

	// Create a state with thousands of builds
	const numBuilds = 10000
	t.Logf("Creating state with %d builds...", numBuilds)

	start := time.Now()
	for i := 0; i < numBuilds; i++ {
		buildName := fmt.Sprintf("build-%05d", i)

		// Create a build with provisioners and artifacts
		build := &Build{
			Name:   buildName,
			Type:   "amazon-ebs",
			Status: BuildStatusComplete,
			Instance: &Instance{
				ID:         fmt.Sprintf("i-%016x", i),
				BuilderID:  "amazon-ebs",
				Provider:   "aws",
				Region:     "us-east-1",
				PublicIP:   fmt.Sprintf("10.0.%d.%d", i/256, i%256),
				PrivateIP:  fmt.Sprintf("172.16.%d.%d", i/256, i%256),
				SSHUser:    "ubuntu",
				SSHPort:    22,
				CreatedAt:  time.Now(),
				Metadata:   map[string]interface{}{"key": "value", "index": i},
			},
			Provisioners: []ProvisionerState{
				{Type: "shell", Status: StatusComplete},
				{Type: "ansible", Status: StatusComplete},
				{Type: "file", Status: StatusComplete},
			},
			Artifacts: []ArtifactState{
				{
					ID:        fmt.Sprintf("ami-%016x", i),
					BuilderID: "amazon-ebs",
					Type:      "ami",
					Files:     []string{"/tmp/artifact1", "/tmp/artifact2"},
					Metadata:  map[string]interface{}{"size": i * 1024},
				},
			},
			StartedAt:   time.Now().Add(-time.Hour),
			CompletedAt: time.Now(),
		}

		state.SetBuild(buildName, build)
	}

	createDuration := time.Since(start)
	t.Logf("Created %d builds in %v", numBuilds, createDuration)

	// Save
	start = time.Now()
	if err := state.Save(statePath); err != nil {
		t.Fatalf("Failed to save large state: %v", err)
	}
	saveDuration := time.Since(start)
	t.Logf("Saved large state in %v", saveDuration)

	// Check file size
	info, err := os.Stat(statePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("State file size: %.2f MB", float64(info.Size())/(1024*1024))

	// Load it back
	start = time.Now()
	loadedState, err := Load(statePath)
	if err != nil {
		t.Fatalf("Failed to load large state: %v", err)
	}
	loadDuration := time.Since(start)
	t.Logf("Loaded large state in %v", loadDuration)

	if len(loadedState.Builds) != numBuilds {
		t.Fatalf("Expected %d builds, got %d", numBuilds, len(loadedState.Builds))
	}

	// Verify some random builds
	for i := 0; i < 100; i++ {
		idx := rand.Intn(numBuilds)
		buildName := fmt.Sprintf("build-%05d", idx)
		build := loadedState.GetBuild(buildName)
		if build == nil {
			t.Fatalf("Build %s not found", buildName)
		}
		if build.Instance == nil {
			t.Fatalf("Build %s has no instance", buildName)
		}
		if len(build.Provisioners) != 3 {
			t.Fatalf("Build %s has wrong provisioner count", buildName)
		}
	}

	t.Logf("Performance: create=%v, save=%v, load=%v", createDuration, saveDuration, loadDuration)
}

// TestStateStress_RapidSaveLoad tests rapid save/load cycles
func TestStateStress_RapidSaveLoad(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "state-rapid-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	statePath := filepath.Join(tmpDir, "rapid.json")

	const iterations = 1000
	t.Logf("Running %d rapid save/load cycles...", iterations)

	start := time.Now()
	for i := 0; i < iterations; i++ {
		state := New(fmt.Sprintf("/tmp/template-%d.pkr.hcl", i))

		// Add some builds
		for j := 0; j < 10; j++ {
			state.SetBuild(fmt.Sprintf("build-%d-%d", i, j), &Build{
				Name:   fmt.Sprintf("build-%d-%d", i, j),
				Type:   "null",
				Status: BuildStatusComplete,
			})
		}

		// Save
		if err := state.Save(statePath); err != nil {
			t.Fatalf("Iteration %d: save failed: %v", i, err)
		}

		// Load immediately
		loaded, err := Load(statePath)
		if err != nil {
			t.Fatalf("Iteration %d: load failed: %v", i, err)
		}

		if len(loaded.Builds) != 10 {
			t.Fatalf("Iteration %d: expected 10 builds, got %d", i, len(loaded.Builds))
		}
	}

	duration := time.Since(start)
	avgCycle := duration / iterations
	t.Logf("Completed %d cycles in %v (avg: %v per cycle)", iterations, duration, avgCycle)
}

// TestStateStress_CorruptionRecovery tests handling of corrupted state files
func TestStateStress_CorruptionRecovery(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "state-corrupt-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	testCases := []struct {
		name    string
		content string
		wantErr bool
	}{
		{
			name:    "empty file",
			content: "",
			wantErr: true,
		},
		{
			name:    "invalid json",
			content: "{invalid json",
			wantErr: true,
		},
		{
			name:    "truncated json",
			content: `{"version": 1, "serial": 1, "builds": {`,
			wantErr: true,
		},
		{
			name:    "null bytes",
			content: string([]byte{0, 0, 0, 0}),
			wantErr: true,
		},
		{
			name:    "valid but minimal",
			content: `{"version": 1, "serial": 1, "lineage": "test", "template": {}, "builds": {}}`,
			wantErr: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			statePath := filepath.Join(tmpDir, tc.name+".json")

			// Write corrupted content
			if err := ioutil.WriteFile(statePath, []byte(tc.content), 0644); err != nil {
				t.Fatal(err)
			}

			// Try to load
			state, err := Load(statePath)

			if tc.wantErr {
				if err == nil {
					t.Errorf("Expected error but got none (state=%v)", state)
				}
			} else {
				if err != nil {
					t.Errorf("Expected success but got error: %v", err)
				}
			}
		})
	}
}

// TestStateStress_DeepNesting tests deeply nested state structures
func TestStateStress_DeepNesting(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "state-deep-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	statePath := filepath.Join(tmpDir, "deep.json")
	state := New("/tmp/template.pkr.hcl")

	// Create a build with deeply nested metadata
	metadata := make(map[string]interface{})
	current := metadata

	// Create 100 levels of nesting
	for i := 0; i < 100; i++ {
		next := make(map[string]interface{})
		current[fmt.Sprintf("level_%d", i)] = next
		current["data"] = fmt.Sprintf("value_%d", i)
		current = next
	}

	build := &Build{
		Name:   "deep-build",
		Type:   "test",
		Status: BuildStatusComplete,
		Instance: &Instance{
			ID:       "test-instance",
			Metadata: metadata,
		},
	}

	state.SetBuild("deep-build", build)

	// Save and load
	if err := state.Save(statePath); err != nil {
		t.Fatalf("Failed to save deeply nested state: %v", err)
	}

	loaded, err := Load(statePath)
	if err != nil {
		t.Fatalf("Failed to load deeply nested state: %v", err)
	}

	loadedBuild := loaded.GetBuild("deep-build")
	if loadedBuild == nil {
		t.Fatal("Build not found after load")
	}

	// Verify we can access nested data
	m := loadedBuild.Instance.Metadata
	for i := 0; i < 100; i++ {
		if m == nil {
			t.Fatalf("Metadata is nil at level %d", i)
		}
		next, ok := m[fmt.Sprintf("level_%d", i)]
		if !ok {
			break
		}
		if nextMap, ok := next.(map[string]interface{}); ok {
			m = nextMap
		}
	}

	t.Logf("Successfully handled deeply nested metadata")
}

// TestStateStress_UnicodeAndSpecialChars tests handling of various character sets
func TestStateStress_UnicodeAndSpecialChars(t *testing.T) {
	tmpDir, err := ioutil.TempDir("", "state-unicode-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	statePath := filepath.Join(tmpDir, "unicode.json")
	state := New("/tmp/template.pkr.hcl")

	// Test various special characters
	testStrings := []string{
		"Hello, 世界",                    // Chinese
		"Привет, мир",                  // Russian
		"مرحبا بالعالم",                // Arabic
		"🚀🎉💻🔧",                       // Emojis
		"<script>alert('xss')</script>", // HTML/XSS
		"'; DROP TABLE builds;--",      // SQL injection
		"\x00\x01\x02",                 // Control chars
		strings.Repeat("A", 10000),     // Very long string
		"",                             // Empty
		"   ",                          // Whitespace
		"\n\r\t",                       // Newlines/tabs
	}

	for i, str := range testStrings {
		build := &Build{
			Name:   fmt.Sprintf("build-%d", i),
			Type:   str,
			Status: BuildStatusComplete,
			Instance: &Instance{
				ID: str,
				Metadata: map[string]interface{}{
					"test": str,
				},
			},
		}
		state.SetBuild(fmt.Sprintf("build-%d", i), build)
	}

	// Save and load
	if err := state.Save(statePath); err != nil {
		t.Fatalf("Failed to save state with special chars: %v", err)
	}

	loaded, err := Load(statePath)
	if err != nil {
		t.Fatalf("Failed to load state with special chars: %v", err)
	}

	// Verify all strings are preserved
	for i, expected := range testStrings {
		build := loaded.GetBuild(fmt.Sprintf("build-%d", i))
		if build == nil {
			t.Fatalf("Build %d not found", i)
		}
		if build.Type != expected {
			t.Errorf("Build %d: type mismatch. Expected %q, got %q", i, expected, build.Type)
		}
	}

	t.Logf("Successfully handled %d different special character scenarios", len(testStrings))
}

// BenchmarkState_Save benchmarks state saving
func BenchmarkState_Save(b *testing.B) {
	tmpDir, err := ioutil.TempDir("", "bench-save-*")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	state := New("/tmp/template.pkr.hcl")

	// Create state with 100 builds
	for i := 0; i < 100; i++ {
		state.SetBuild(fmt.Sprintf("build-%d", i), &Build{
			Name:   fmt.Sprintf("build-%d", i),
			Type:   "amazon-ebs",
			Status: BuildStatusComplete,
		})
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		statePath := filepath.Join(tmpDir, fmt.Sprintf("bench-%d.json", i))
		if err := state.Save(statePath); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkState_Load benchmarks state loading
func BenchmarkState_Load(b *testing.B) {
	tmpDir, err := ioutil.TempDir("", "bench-load-*")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create and save a state
	statePath := filepath.Join(tmpDir, "bench.json")
	state := New("/tmp/template.pkr.hcl")

	for i := 0; i < 100; i++ {
		state.SetBuild(fmt.Sprintf("build-%d", i), &Build{
			Name:   fmt.Sprintf("build-%d", i),
			Type:   "amazon-ebs",
			Status: BuildStatusComplete,
		})
	}

	if err := state.Save(statePath); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := Load(statePath)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkState_ConcurrentAccess benchmarks concurrent state operations
func BenchmarkState_ConcurrentAccess(b *testing.B) {
	state := New("/tmp/template.pkr.hcl")

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			buildName := fmt.Sprintf("build-%d", i%100)

			// Mix of reads and writes
			if i%3 == 0 {
				state.SetBuild(buildName, &Build{
					Name:   buildName,
					Type:   "test",
					Status: BuildStatusComplete,
				})
			} else {
				_ = state.GetBuild(buildName)
			}
			i++
		}
	})
}
