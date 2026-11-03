package receivers

import (
	"saundo/internal/settings"
)

// MigrationResult reports what the settings migration did, for the log.
type MigrationResult struct {
	MovedPasswords int
	KeptPasswords  int
	SeededAuto     bool
}

// MigrateSettings is the idempotent startup migration from the legacy
// per-receiver state in settings.json. It
//
//   - moves every plaintext password into the DPAPI credentials store under
//     the "password:" namespace; one that fails to save stays in the
//     settings copy rather than being lost, and is reported as kept;
//   - seeds exactly one auto-connect entry from the legacy AutoConnect flag
//     and LastDeviceID, and clears the flag so the migration never runs
//     twice.
//
// It reads settings.json and writes receivers.json plus the DPAPI store;
// settings.json keeps its legacy fields decodable but production no longer
// reads them. The caller owns the one-writer sequencing: run it before the
// engine starts. A corrupt receivers.json skips the seed (the passwords
// still move); the legacy flag stays set so the next healthy start retries.
func MigrateSettings(sets *settings.Store, secret PasswordStore, reg *Store) (MigrationResult, error) {
	var res MigrationResult
	cfg := sets.Load()

	// The legacy map keys are device IDs. Namespace them so the file's
	// other entries (HAP keys) and these can never collide. A password
	// already in DPAPI wins: the plaintext copy may be older, so it is
	// dropped rather than written over the newer secret.
	for id, pw := range cfg.Passwords {
		if pw == "" {
			delete(cfg.Passwords, id)
			continue
		}
		if secret.HasPassword(id) {
			delete(cfg.Passwords, id)
			res.MovedPasswords++
			continue
		}
		if err := secret.SavePassword(id, pw); err != nil {
			res.KeptPasswords++
			continue
		}
		delete(cfg.Passwords, id)
		res.MovedPasswords++
	}
	if cfg.AutoConnect {
		if cfg.LastDeviceID == "" {
			// Nothing to seed; clear the legacy switch so it is not read again.
			cfg.AutoConnect = false
		} else {
			stored, err := reg.Load()
			if err == nil {
				e, ok := stored.Get(cfg.LastDeviceID)
				if !ok {
					e = Entry{Key: cfg.LastDeviceID, Kind: KindDiscovered}
				}
				e.AutoConnect = true
				stored.Put(e)
				stored.SingleAutoConnect(cfg.LastDeviceID, true)
				if err := reg.Save(stored); err == nil {
					cfg.AutoConnect = false
					res.SeededAuto = true
				}
			}
		}
	}
	if err := sets.Save(cfg); err != nil {
		return res, err
	}
	return res, nil
}

// PasswordStore is the DPAPI namespace the passwords live in. The
// credentials store implements it; tests can fake it.
type PasswordStore interface {
	SavePassword(key, password string) error
	LoadPassword(key string) string
	DeletePassword(key string) error
	HasPassword(key string) bool
}
