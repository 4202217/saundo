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
// passwords. A file already in the new directory wins. Each copy lands via
// a tmp file and a rename, so a failed or partial write never leaves a
// truncated destination that the next start would take as done.
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
		writeLegacyCopy(dst, data)
	}
}

// writeLegacyCopy writes dst atomically: the payload lands in a tmp file
// first, and a failed write removes it instead of leaving a truncated
// destination or litter behind.
func writeLegacyCopy(dst string, data []byte) {
	tmp := dst + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		_ = os.Remove(tmp)
		return
	}
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
	}
}
