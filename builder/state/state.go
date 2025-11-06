package state

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
)

// State represents the complete builder state file
type State struct {
	Version        int               `json:"version"`
	Serial         int               `json:"serial"`
	Lineage        string            `json:"lineage"`
	BuilderVersion string            `json:"builder_version"`
	PackerVersion  string            `json:"packer_version"`
	Template       TemplateState     `json:"template"`
	Builds         map[string]*Build `json:"builds"`
	LastRun        *RunInfo          `json:"last_run,omitempty"`

	mu       sync.RWMutex `json:"-"`
	filePath string       `json:"-"`
}

// TemplateState tracks the template and its inputs
type TemplateState struct {
	Path      string            `json:"path"`
	Hash      string            `json:"hash"`
	Variables map[string]string `json:"variables"`
	Files     map[string]string `json:"files"` // path -> hash of source files
}

// Build represents a single build's state
type Build struct {
	Name         string              `json:"name"`
	Type         string              `json:"type"`
	Status       BuildStatus         `json:"status"`
	Instance     *Instance           `json:"instance,omitempty"`
	Provisioners []ProvisionerState  `json:"provisioners"`
	PostProcess  []PostProcessorState `json:"post_processors,omitempty"`
	Artifacts    []ArtifactState     `json:"artifacts,omitempty"`
	Error        string              `json:"error,omitempty"`
	StartedAt    time.Time           `json:"started_at,omitempty"`
	CompletedAt  time.Time           `json:"completed_at,omitempty"`
}

// Instance represents a VM/container instance
type Instance struct {
	ID              string                 `json:"id"`
	BuilderID       string                 `json:"builder_id"`
	Provider        string                 `json:"provider"`
	Region          string                 `json:"region,omitempty"`
	PublicIP        string                 `json:"public_ip,omitempty"`
	PrivateIP       string                 `json:"private_ip,omitempty"`
	SSHUser         string                 `json:"ssh_user,omitempty"`
	SSHPort         int                    `json:"ssh_port,omitempty"`
	SSHKeyPath      string                 `json:"ssh_key_path,omitempty"`
	WinRMUser       string                 `json:"winrm_user,omitempty"`
	WinRMPort       int                    `json:"winrm_port,omitempty"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
	CreatedAt       time.Time              `json:"created_at"`
	KeepOnFailure   bool                   `json:"keep_on_failure"`
}

// ProvisionerState tracks provisioner execution
type ProvisionerState struct {
	Type      string    `json:"type"`
	Name      string    `json:"name,omitempty"`
	Status    Status    `json:"status"`
	Error     string    `json:"error,omitempty"`
	StartedAt time.Time `json:"started_at,omitempty"`
	EndedAt   time.Time `json:"ended_at,omitempty"`
}

// PostProcessorState tracks post-processor execution
type PostProcessorState struct {
	Type      string    `json:"type"`
	Name      string    `json:"name,omitempty"`
	Status    Status    `json:"status"`
	Error     string    `json:"error,omitempty"`
	StartedAt time.Time `json:"started_at,omitempty"`
	EndedAt   time.Time `json:"ended_at,omitempty"`
}

// ArtifactState tracks generated artifacts
type ArtifactState struct {
	ID        string                 `json:"id"`
	BuilderID string                 `json:"builder_id"`
	Type      string                 `json:"type"`
	Files     []string               `json:"files,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
	Hash      string                 `json:"hash,omitempty"`
}

// RunInfo tracks the last run
type RunInfo struct {
	StartedAt   time.Time `json:"started_at"`
	CompletedAt time.Time `json:"completed_at,omitempty"`
}

// BuildStatus represents the overall build status
type BuildStatus string

const (
	BuildStatusPending    BuildStatus = "pending"
	BuildStatusCreating   BuildStatus = "creating"
	BuildStatusProvisioning BuildStatus = "provisioning"
	BuildStatusPostProcessing BuildStatus = "post_processing"
	BuildStatusComplete   BuildStatus = "complete"
	BuildStatusFailed     BuildStatus = "failed"
)

// Status represents execution status
type Status string

const (
	StatusPending  Status = "pending"
	StatusRunning  Status = "running"
	StatusComplete Status = "complete"
	StatusFailed   Status = "failed"
	StatusSkipped  Status = "skipped"
)

// New creates a new empty state
func New(templatePath string) *State {
	return &State{
		Version:  1,
		Serial:   1,
		Lineage:  uuid.New().String(),
		Template: TemplateState{
			Path:      templatePath,
			Variables: make(map[string]string),
			Files:     make(map[string]string),
		},
		Builds: make(map[string]*Build),
	}
}

// Load loads state from a file
func Load(path string) (*State, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No state file yet
		}
		return nil, fmt.Errorf("failed to open state file: %w", err)
	}
	defer f.Close()

	var state State
	if err := json.NewDecoder(f).Decode(&state); err != nil {
		return nil, fmt.Errorf("failed to decode state file: %w", err)
	}

	state.filePath = path
	return &state, nil
}

// Save saves state to a file
func (s *State) Save(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Serial++
	s.filePath = path

	// Create directory if it doesn't exist
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create state directory: %w", err)
	}

	// Write to temp file first
	tmpPath := path + ".tmp"
	f, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("failed to create temp state file: %w", err)
	}

	encoder := json.NewEncoder(f)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(s); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("failed to encode state: %w", err)
	}

	if err := f.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to close temp state file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to rename state file: %w", err)
	}

	return nil
}

// GetBuild returns the build state for a given name
func (s *State) GetBuild(name string) *Build {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Builds[name]
}

// SetBuild sets the build state for a given name
func (s *State) SetBuild(name string, build *Build) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Builds[name] = build
}

// RemoveBuild removes a build from state
func (s *State) RemoveBuild(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.Builds, name)
}

// ComputeFingerprint computes a fingerprint of the template and inputs
func (s *State) ComputeFingerprint() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	h := sha256.New()

	// Include template hash
	io.WriteString(h, s.Template.Hash)

	// Include sorted variables
	for k, v := range s.Template.Variables {
		io.WriteString(h, k)
		io.WriteString(h, v)
	}

	// Include sorted file hashes
	for k, v := range s.Template.Files {
		io.WriteString(h, k)
		io.WriteString(h, v)
	}

	return fmt.Sprintf("sha256:%x", h.Sum(nil))
}

// IsComplete checks if a build is complete
func (b *Build) IsComplete() bool {
	return b.Status == BuildStatusComplete
}

// HasInstance checks if build has an instance
func (b *Build) HasInstance() bool {
	return b.Instance != nil && b.Instance.ID != ""
}

// ProvisionerComplete checks if a provisioner is complete
func (b *Build) ProvisionerComplete(index int) bool {
	if index >= len(b.Provisioners) {
		return false
	}
	return b.Provisioners[index].Status == StatusComplete
}

// NextPendingProvisioner returns the index of the next pending provisioner
func (b *Build) NextPendingProvisioner() int {
	for i, p := range b.Provisioners {
		if p.Status == StatusPending || p.Status == StatusFailed {
			return i
		}
	}
	return len(b.Provisioners)
}
