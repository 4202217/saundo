package app

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// configDirForTest points os.UserConfigDir at a temporary root and returns the
// directory the app will use under it.
func configDirForTest(t *testing.T, root string) string {
	t.Helper()
	switch runtime.GOOS {
	case "windows":
		t.Setenv("APPDATA", root)
		return root
	case "darwin":
		t.Setenv("HOME", root)
		return filepath.Join(root, "Library", "Application Support")
	default:
		t.Setenv("XDG_CONFIG_HOME", root)
		return root
	}
}

// TestMigrateLegacyDataCopiesWithoutOverwriting pins the rename migration:
// both files move from the old directory once, and an existing new file wins.
func TestMigrateLegacyDataCopiesWithoutOverwriting(t *testing.T) {
	configDir := configDirForTest(t, t.TempDir())
	oldDir := filepath.Join(configDir, legacyAppDir)
	newDir := filepath.Join(configDir, "saundo")
	if err := os.MkdirAll(oldDir, 0o700); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"settings.json":   `{"name":"legacy"}`,
		"credentials.dat": "protected",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(oldDir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	migrateLegacyData()

	for name, body := range files {
		got, err := os.ReadFile(filepath.Join(newDir, name))
		if err != nil {
			t.Fatalf("%s not copied: %v", name, err)
		}
		if string(got) != body {
			t.Fatalf("%s = %q, want %q", name, got, body)
		}
	}

	if err := os.WriteFile(filepath.Join(newDir, "settings.json"), []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(oldDir, "settings.json"), []byte("legacy-again"), 0o600); err != nil {
		t.Fatal(err)
	}
	migrateLegacyData()
	got, err := os.ReadFile(filepath.Join(newDir, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new" {
		t.Fatalf("existing new settings overwritten with %q", got)
	}
}

// TestMigrateLegacyDataWithoutOldDirectoryIsANoOp keeps a fresh install from
// creating state it does not need.
func TestMigrateLegacyDataWithoutOldDirectoryIsANoOp(t *testing.T) {
	configDir := configDirForTest(t, t.TempDir())
	migrateLegacyData()
	if _, err := os.Stat(filepath.Join(configDir, "saundo")); !os.IsNotExist(err) {
		t.Fatalf("migration created the new directory without an old one: %v", err)
	}
}
