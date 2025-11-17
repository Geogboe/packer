package state

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Manager handles state file operations with locking
type Manager struct {
	statePath   string
	lockManager *LockManager
	state       *State
}

// NewManager creates a new state manager
func NewManager(statePath string) *Manager {
	return &Manager{
		statePath:   statePath,
		lockManager: NewLockManager(statePath),
	}
}

// DefaultStatePath returns the default state file path
func DefaultStatePath(templateDir string) string {
	return filepath.Join(templateDir, ".packer.d", "builder-state.json")
}

// Load loads and locks the state file
func (m *Manager) Load() (*State, error) {
	// Lock the state
	if err := m.lockManager.Lock("build"); err != nil {
		return nil, fmt.Errorf("failed to lock state: %w", err)
	}

	// Load state
	state, err := Load(m.statePath)
	if err != nil {
		m.lockManager.Unlock()
		return nil, err
	}

	if state == nil {
		// No existing state, create new
		state = New(m.statePath)
	}

	m.state = state
	return state, nil
}

// Save saves the state file
func (m *Manager) Save() error {
	if m.state == nil {
		return fmt.Errorf("no state loaded")
	}

	return m.state.Save(m.statePath)
}

// Unlock unlocks the state file
func (m *Manager) Unlock() error {
	return m.lockManager.Unlock()
}

// Close saves and unlocks the state
func (m *Manager) Close() error {
	if m.state != nil {
		if err := m.Save(); err != nil {
			return err
		}
	}
	return m.Unlock()
}

// State returns the current state
func (m *Manager) State() *State {
	return m.state
}

// ComputeFileHash computes the SHA256 hash of a file
func ComputeFileHash(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return fmt.Sprintf("sha256:%x", h.Sum(nil)), nil
}

// ComputeStringHash computes the SHA256 hash of a string
func ComputeStringHash(s string) string {
	h := sha256.New()
	io.WriteString(h, s)
	return fmt.Sprintf("sha256:%x", h.Sum(nil))
}

// InputsChanged checks if template inputs have changed
func (m *Manager) InputsChanged(templateHash string, variables map[string]string, files map[string]string) bool {
	if m.state == nil {
		return true
	}

	// Check template hash
	if m.state.Template.Hash != templateHash {
		return true
	}

	// Check variables
	if len(m.state.Template.Variables) != len(variables) {
		return true
	}
	for k, v := range variables {
		if m.state.Template.Variables[k] != v {
			return true
		}
	}

	// Check files
	if len(m.state.Template.Files) != len(files) {
		return true
	}
	for k, v := range files {
		if m.state.Template.Files[k] != v {
			return true
		}
	}

	return false
}

// UpdateTemplateInputs updates the template inputs in state
func (m *Manager) UpdateTemplateInputs(templatePath, templateHash string, variables map[string]string, files map[string]string) {
	if m.state == nil {
		return
	}

	m.state.Template.Path = templatePath
	m.state.Template.Hash = templateHash
	m.state.Template.Variables = variables
	m.state.Template.Files = files
}
