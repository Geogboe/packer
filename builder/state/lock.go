package state

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

// Lock represents a state file lock
type Lock struct {
	ID        string    `json:"id"`
	Operation string    `json:"operation"`
	Who       string    `json:"who"`
	Created   time.Time `json:"created"`
	Path      string    `json:"path"`
}

// LockManager handles state file locking
type LockManager struct {
	statePath string
	lockPath  string
	lock      *Lock
}

// NewLockManager creates a new lock manager
func NewLockManager(statePath string) *LockManager {
	lockPath := statePath + ".lock"
	return &LockManager{
		statePath: statePath,
		lockPath:  lockPath,
	}
}

// Lock acquires a lock on the state file
func (lm *LockManager) Lock(operation string) error {
	// Check if lock already exists
	if _, err := os.Stat(lm.lockPath); err == nil {
		// Lock file exists, try to read it
		existingLock, err := lm.readLock()
		if err != nil {
			return fmt.Errorf("failed to read existing lock: %w", err)
		}
		return fmt.Errorf("state is locked by %s (ID: %s, Operation: %s, Created: %s)",
			existingLock.Who, existingLock.ID, existingLock.Operation, existingLock.Created)
	}

	// Create lock
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "unknown"
	}

	lock := &Lock{
		ID:        uuid.New().String(),
		Operation: operation,
		Who:       fmt.Sprintf("%s@%s", os.Getenv("USER"), hostname),
		Created:   time.Now(),
		Path:      lm.statePath,
	}

	// Write lock file
	lockData, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal lock: %w", err)
	}

	// Create directory if needed
	dir := filepath.Dir(lm.lockPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create lock directory: %w", err)
	}

	// Write atomically with O_EXCL to prevent race conditions
	f, err := os.OpenFile(lm.lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		if os.IsExist(err) {
			// Someone beat us to it
			existingLock, _ := lm.readLock()
			if existingLock != nil {
				return fmt.Errorf("state is locked by %s (ID: %s)", existingLock.Who, existingLock.ID)
			}
			return fmt.Errorf("state is locked")
		}
		return fmt.Errorf("failed to create lock file: %w", err)
	}

	if _, err := f.Write(lockData); err != nil {
		f.Close()
		os.Remove(lm.lockPath)
		return fmt.Errorf("failed to write lock file: %w", err)
	}

	if err := f.Close(); err != nil {
		os.Remove(lm.lockPath)
		return fmt.Errorf("failed to close lock file: %w", err)
	}

	lm.lock = lock
	return nil
}

// Unlock releases the lock
func (lm *LockManager) Unlock() error {
	if lm.lock == nil {
		return nil // No lock held
	}

	// Verify we still own the lock
	existingLock, err := lm.readLock()
	if err != nil {
		return fmt.Errorf("failed to read lock before unlock: %w", err)
	}

	if existingLock.ID != lm.lock.ID {
		return fmt.Errorf("lock was stolen by %s (ID: %s)", existingLock.Who, existingLock.ID)
	}

	// Remove lock file
	if err := os.Remove(lm.lockPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove lock file: %w", err)
	}

	lm.lock = nil
	return nil
}

// readLock reads the lock file
func (lm *LockManager) readLock() (*Lock, error) {
	data, err := os.ReadFile(lm.lockPath)
	if err != nil {
		return nil, err
	}

	var lock Lock
	if err := json.Unmarshal(data, &lock); err != nil {
		return nil, err
	}

	return &lock, nil
}

// ForceUnlock forcibly removes a lock (dangerous!)
func (lm *LockManager) ForceUnlock() error {
	if err := os.Remove(lm.lockPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to force unlock: %w", err)
	}
	lm.lock = nil
	return nil
}
