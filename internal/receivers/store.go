package receivers

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// ErrCorrupt reports a receivers.json this build cannot parse. The store
// then refuses to save, so one bad file cannot be silently destroyed; the
// engine keeps working from its in-memory copy and logs.
var ErrCorrupt = errors.New("receivers: corrupt registry file")

// Store owns receivers.json. In production only the engine goroutine calls
// it; the startup migration runs before the engine exists, so there is
// always one writer. The engine holds the authoritative in-memory registry
// and saves it whole.
type Store struct {
	mu      sync.Mutex
	path    string
	corrupt bool
}

// NewStore returns the per-user registry store.
func NewStore() *Store {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	return &Store{path: filepath.Join(dir, "saundo", "receivers.json")}
}

// NewStoreAt is the testable form of NewStore.
func NewStoreAt(path string) *Store { return &Store{path: path} }

// Load returns the stored registry. A missing file is an empty registry; an
// unreadable or corrupt one is an error, never silently empty — the caller
// decides the fail-closed path.
func (s *Store) Load() (Registry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	reg, err := s.load()
	if err == nil {
		s.corrupt = false
	}
	return reg, err
}

func (s *Store) load() (Registry, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return Registry{Version: SchemaVersion}, nil
		}
		s.corrupt = true
		return Registry{Version: SchemaVersion}, fmt.Errorf("receivers: read: %w", err)
	}
	var reg Registry
	if err := json.Unmarshal(data, &reg); err != nil {
		s.corrupt = true
		return Registry{Version: SchemaVersion}, fmt.Errorf("%w: %v", ErrCorrupt, err)
	}
	if reg.Version > SchemaVersion {
		// Do not let this build overwrite a document written by a newer one.
		s.corrupt = true
		return Registry{Version: SchemaVersion}, fmt.Errorf("receivers: schema %d is newer than build's %d", reg.Version, SchemaVersion)
	}
	// A dropped key is an unnamed receiver; keep the row manageable.
	for i := range reg.Entries {
		if reg.Entries[i].Key == "" {
			reg.Entries[i].Key = manualKeyPrefix + "invalid"
		}
		if reg.Entries[i].Kind == "" {
			reg.Entries[i].Kind = KindDiscovered
		}
		reg.Entries[i].DelayMS = ClampedDelay(reg.Entries[i].DelayMS)
	}
	return reg, nil
}

// Save writes the registry atomically. It refuses to write once the file
// has failed to parse, so the user's rows are never destroyed by an
// unreadable state; a later successful Load clears the guard.
func (s *Store) Save(reg Registry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.corrupt {
		return ErrCorrupt
	}
	reg.Version = SchemaVersion
	data, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
