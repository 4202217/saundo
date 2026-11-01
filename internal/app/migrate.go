package app

import (
	"os"
	"path/filepath"
)

// The app stored its data under the old application name until the rename
// to saundo. These two strings are the only place that name remains.
const (
	legacyAppDir = "airgoplay"
	legacyName   = legacyAppDir
)

// migrateLegacyData copies the settings and credentials written under the
// old name, once, so the rename keeps the last device and the saved
// passwords. A file already in the new directory wins.
func migrateLegacyData() {
	dir, err := os.UserConfigDir()
	if err != nil {
		return
	}
	oldDir := filepath.Join(dir, legacyAppDir)
	newDir := filepath.Join(dir, "saundo")
	for _, name := range []string{"settings.json", "credentials.dat"} {
		dst := filepath.Join(newDir, name)
		if _, err := os.Stat(dst); err == nil {
			continue
		}
		data, err := os.ReadFile(filepath.Join(oldDir, name))
		if err != nil {
			continue
		}
		if err := os.MkdirAll(newDir, 0o700); err != nil {
			return
		}
		_ = os.WriteFile(dst, data, 0o600)
	}
}
