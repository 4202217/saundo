// Package creds carries the HAP long-term pairing credentials and persists
// them encrypted with Windows DPAPI.
package creds

import (
	"encoding/hex"
	"encoding/json"
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

// Load returns the stored JSON for deviceID, or "".
func (s *Store) Load(deviceID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	all := s.readAll()
	return all[deviceID]
}

// Save stores the JSON for deviceID.
func (s *Store) Save(deviceID, credsJSON string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	all := s.readAll()
	all[deviceID] = credsJSON
	return s.writeAll(all)
}

// Delete removes a device's credentials.
func (s *Store) Delete(deviceID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	all := s.readAll()
	delete(all, deviceID)
	return s.writeAll(all)
}

func (s *Store) readAll() map[string]string {
	all := make(map[string]string)
	blob, err := os.ReadFile(s.path)
	if err != nil {
		return all
	}
	plain, err := unprotect(blob)
	if err != nil {
		return all
	}
	_ = json.Unmarshal(plain, &all)
	return all
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
