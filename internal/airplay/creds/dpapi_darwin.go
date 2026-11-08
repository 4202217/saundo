//go:build darwin

package creds

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"

	"saundo/internal/darwin"
)

// At-rest protection on macOS: AES-256-GCM with a per-user master key held
// in the login Keychain. One key per user matches Windows DPAPI's scope; the
// file-based keychain needs no code signing.

const (
	keychainService = "saundo"
	keychainAccount = "credentials master key"

	nonceSize = 12
	keySize   = 32
	tagSize   = 16
)

// envMasterKey overrides the keychain master key from the environment, for
// headless CI runs where no keychain prompt can be answered. It is never set
// by the app itself.
const envMasterKey = "SAUNDO_CREDS_MASTER_KEY"

func masterKey() ([]byte, error) {
	if h := os.Getenv(envMasterKey); h != "" {
		key, err := hex.DecodeString(h)
		if err != nil || len(key) != keySize {
			// A malformed override fails closed instead of silently
			// falling back to the keychain.
			return nil, fmt.Errorf("creds: %s must be %d hex bytes", envMasterKey, keySize)
		}
		return key, nil
	}
	key, err := darwin.KeychainGet(keychainService, keychainAccount)
	if errors.Is(err, darwin.ErrKeychainItemNotFound) {
		key = make([]byte, keySize)
		if _, err := rand.Read(key); err != nil {
			return nil, fmt.Errorf("creds: generate master key: %w", err)
		}
		if err := darwin.KeychainSet(keychainService, keychainAccount, key); err != nil {
			return nil, fmt.Errorf("creds: store master key: %w", err)
		}
		return key, nil
	}
	if err != nil {
		return nil, fmt.Errorf("creds: read master key: %w", err)
	}
	if len(key) != keySize {
		// A key of any other length is not one this process wrote: refuse
		// rather than silently rekeying over credentials that would then be
		// unreadable.
		return nil, fmt.Errorf("creds: master key has %d bytes, want %d", len(key), keySize)
	}
	return key, nil
}

func gcm(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("creds: cipher: %w", err)
	}
	g, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("creds: gcm: %w", err)
	}
	return g, nil
}

func protect(plain []byte) ([]byte, error) {
	key, err := masterKey()
	if err != nil {
		return nil, err
	}
	g, err := gcm(key)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, nonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("creds: nonce: %w", err)
	}
	return g.Seal(nonce, nonce, plain, nil), nil
}

func unprotect(cipherText []byte) ([]byte, error) {
	if len(cipherText) == 0 {
		return nil, nil
	}
	key, err := masterKey()
	if err != nil {
		return nil, err
	}
	g, err := gcm(key)
	if err != nil {
		return nil, err
	}
	if len(cipherText) < nonceSize+tagSize {
		return nil, errors.New("creds: ciphertext too short")
	}
	plain, err := g.Open(nil, cipherText[:nonceSize], cipherText[nonceSize:], nil)
	if err != nil {
		// A failed decrypt means "unknown": the caller keeps its previous
		// state instead of adopting an empty store.
		return nil, fmt.Errorf("creds: decrypt: %w", err)
	}
	return plain, nil
}
