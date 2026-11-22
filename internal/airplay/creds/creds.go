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
	"time"
)

// Secret is a long-term key. Every formatting verb redacts it, so a stray
// log line or error message cannot print the key.
type Secret []byte

// Format makes %v, %s, %x, and %q all print [redacted].
func (s Secret) Format(f fmt.State, verb rune) {
	_, _ = f.Write([]byte("[redacted]"))
}

// Credentials are the long-term keys from a successful HAP pairing. LTSK is
// our secret, LTPK the receiver's public key, ATVID the receiver's pairing
// identifier, and ClientID our pairing UUID.
type Credentials struct {
	ATVID    []byte `json:"-"`
	ClientID string `json:"-"`
	LTPK     []byte `json:"-"`
	LTSK     Secret `json:"-"`
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
	return Credentials{ATVID: atv, ClientID: w.ClientID, LTPK: ltpk, LTSK: Secret(ltsk)}, true
}

// Store keeps one credentials blob per device, encrypted at rest.
type Store struct {
	mu            sync.Mutex
	path          string
	quarantinedTo string
	// lastGood is the last successfully decrypted map. A failed read is
	// unknown, not empty: loads serve it so a transient I/O failure or a
	// locked Keychain never reads as "unpaired", which would re-pair.
	lastGood map[string]string
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

// TakeQuarantine returns the path an unreadable credentials file was moved to
// since the last call, or "". The store is usable again; the caller should log
// that a receiver may need to be paired again.
func (s *Store) TakeQuarantine() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.quarantinedTo
	s.quarantinedTo = ""
	return p
}

// ErrCorrupt reports a credentials file this process cannot read and cannot
// move aside. Loads fail closed to "", and writes refuse to replace the file,
// so one bad save cannot destroy the keys it cannot decrypt.
var ErrCorrupt = errors.New("creds: unreadable credentials file")

// errKeyUnavailable classifies a decrypt error that comes from the key side,
// not from the blob: a locked or unreadable Keychain, a malformed master-key
// override. Such an error is transient and says nothing about the stored
// blob, so the store keeps the file and fails closed instead of quarantining
// a store that would decrypt fine once the key returns.
var errKeyUnavailable = errors.New("creds: key unavailable")

// unprotectFunc is the swap point tests use to classify decrypt outcomes.
var unprotectFunc = unprotect

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
			s.lastGood = all
			return all, nil
		}
		// A failed read is unknown, not empty: keep the last good map so a
		// transient I/O failure never reads as "unpaired".
		return s.lastGood, fmt.Errorf("creds: read: %w", err)
	}
	plain, err := unprotectFunc(blob)
	if err != nil {
		if errors.Is(err, errKeyUnavailable) {
			// The key side failed, not the blob: the file stays, loads serve
			// the last known state, and writes refuse to run.
			return s.lastGood, err
		}
		return all, s.quarantine(fmt.Errorf("%w: %v", ErrCorrupt, err))
	}
	if err := json.Unmarshal(plain, &all); err != nil {
		return all, s.quarantine(fmt.Errorf("%w: %v", ErrCorrupt, err))
	}
	if all == nil {
		// A plaintext of the JSON literal null unmarshals to a nil map
		// without error; treat it as an empty store so a later write cannot
		// panic.
		all = make(map[string]string)
	}
	s.lastGood = all
	return all, nil
}

// quarantine moves an unreadable credentials file aside and reports nil, so
// the caller can create a fresh file in its place. When the move fails the
// original error comes back and writes keep failing closed.
func (s *Store) quarantine(cause error) error {
	dest := s.path + ".corrupt-" + time.Now().UTC().Format("20060102T150405.000000000Z")
	if err := os.Rename(s.path, dest); err != nil {
		return cause
	}
	s.quarantinedTo = dest
	return nil
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
	if err := writeFileSynced(tmp, blob); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return err
	}
	// The write is the newest truth: a read that fails right after must not
	// serve the map from before it.
	s.lastGood = all
	return nil
}

// writeFileSynced writes and flushes a temp file so a rename survives a
// power cut, not just a process crash.
func writeFileSynced(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}
