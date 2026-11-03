// Package creds carries the HAP long-term pairing credentials and persists
// them encrypted with Windows DPAPI.
package creds

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Credentials are the long-term keys from a successful HAP pairing. LTSK is
// our secret, LTPK the receiver's public key, ATVID the receiver's pairing
// identifier, and ClientID our pairing UUID.
type Credentials struct {
	ATVID    []byte `json:"-"`
	ClientID string `json:"-"`
	LTPK     []byte `json:"-"`
	LTSK     []byte `json:"-"`
}

type wireCreds struct {
	ATVID    string `json:"atvId"`
	ClientID string `json:"clientId"`
	LTPK     string `json:"ltpk"`
	LTSK     string `json:"ltsk"`
}

// Marshal renders the compact JSON the C++ sender and this program share.
func Marshal(c Credentials) string {
	b, _ := json.Marshal(wireCreds{
		ATVID:    hex.EncodeToString(c.ATVID),
		ClientID: c.ClientID,
		LTPK:     hex.EncodeToString(c.LTPK),
		LTSK:     hex.EncodeToString(c.LTSK),
	})
	return string(b)
}

// Parse reads the shared credentials JSON. It rejects malformed hex and
// missing fields.
func Parse(text string) (Credentials, bool) {
	if text == "" {
		return Credentials{}, false
	}
	var w wireCreds
	if err := json.Unmarshal([]byte(text), &w); err != nil {
		return Credentials{}, false
	}
	atv, err1 := hex.DecodeString(w.ATVID)
	ltpk, err2 := hex.DecodeString(w.LTPK)
	ltsk, err3 := hex.DecodeString(w.LTSK)
	if err1 != nil || err2 != nil || err3 != nil || w.ClientID == "" {
		return Credentials{}, false
	}
	return Credentials{ATVID: atv, ClientID: w.ClientID, LTPK: ltpk, LTSK: ltsk}, true
}

// Store keeps one credentials blob per device, encrypted at rest.
type Store struct {
	mu   sync.Mutex
	path string
}

// NewStore returns the per-user credentials store, creating no file yet.
func NewStore() *Store {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	return &Store{path: filepath.Join(dir, "saundo", "credentials.dat")}
}

// NewStoreAt is the testable form of NewStore.
func NewStoreAt(path string) *Store { return &Store{path: path} }

// ErrCorrupt reports a credentials file this process cannot read. Loads fail
// closed to "", and writes refuse to replace the file, so one bad save cannot
// destroy the HAP keys and receiver passwords it cannot decrypt.
var ErrCorrupt = errors.New("creds: unreadable credentials file")

// Load returns the stored JSON for deviceID, or "".
func (s *Store) Load(deviceID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	all, _ := s.readAll()
	return all[deviceID]
}

// Save stores the JSON for deviceID.
func (s *Store) Save(deviceID, credsJSON string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	all, err := s.readAll()
	if err != nil {
		return err
	}
	all[deviceID] = credsJSON
	return s.writeAll(all)
}

// Delete removes a device's credentials.
func (s *Store) Delete(deviceID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	all, err := s.readAll()
	if err != nil {
		return err
	}
	delete(all, deviceID)
	return s.writeAll(all)
}

// passwordKeyPrefix namespaces receiver passwords inside the same DPAPI
// file the HAP long-term keys use. Old builds never look up these keys, so
// the map stays forward and backward compatible.
const passwordKeyPrefix = "password:"

func passwordKey(deviceID string) string { return passwordKeyPrefix + deviceID }

// LoadPassword returns the stored AirPlay receiver password, or "". A
// decrypt failure reads as "", which the engine's password gate treats as
// "prompt the user" — the fail-closed path.
func (s *Store) LoadPassword(deviceID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	all, _ := s.readAll()
	return all[passwordKey(deviceID)]
}

// SavePassword stores the receiver's AirPlay password. An empty password
// deletes it.
func (s *Store) SavePassword(deviceID, password string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	all, err := s.readAll()
	if err != nil {
		return err
	}
	if password == "" {
		delete(all, passwordKey(deviceID))
	} else {
		all[passwordKey(deviceID)] = password
	}
	return s.writeAll(all)
}

// DeletePassword removes the receiver's stored password.
func (s *Store) DeletePassword(deviceID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	all, err := s.readAll()
	if err != nil {
		return err
	}
	delete(all, passwordKey(deviceID))
	return s.writeAll(all)
}

// HasPassword reports whether a password exists for the receiver. The
// engine keeps this projection so the merge does not decrypt DPAPI per row.
func (s *Store) HasPassword(deviceID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	all, err := s.readAll()
	if err != nil {
		return false
	}
	_, ok := all[passwordKey(deviceID)]
	return ok
}

func (s *Store) readAll() (map[string]string, error) {
	all := make(map[string]string)
	blob, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return all, nil
		}
		return all, fmt.Errorf("creds: read: %w", err)
	}
	plain, err := unprotect(blob)
	if err != nil {
		return all, fmt.Errorf("%w: %v", ErrCorrupt, err)
	}
	if err := json.Unmarshal(plain, &all); err != nil {
		return all, fmt.Errorf("%w: %v", ErrCorrupt, err)
	}
	return all, nil
}

func (s *Store) writeAll(all map[string]string) error {
	plain, err := json.Marshal(all)
	if err != nil {
		return err
	}
	blob, err := protect(plain)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, blob, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
