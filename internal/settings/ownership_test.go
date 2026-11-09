package settings

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestUpdatePreservesFieldsItDoesNotTouch(t *testing.T) {
	sets := NewStoreAt(filepath.Join(t.TempDir(), "settings.json"))
	cfg := Default()
	cfg.Beta = true
	cfg.Volume = 42
	if err := sets.Save(cfg); err != nil {
		t.Fatal(err)
	}

	if err := sets.Update(func(s *Settings) { s.Theme = ThemeDark }); err != nil {
		t.Fatal(err)
	}

	got := sets.Load()
	if got.Theme != ThemeDark {
		t.Fatalf("theme = %q, want dark", got.Theme)
	}
	if !got.Beta {
		t.Fatal("update dropped the channel")
	}
	if got.Volume != 42 {
		t.Fatalf("update dropped the volume: %v", got.Volume)
	}
}

// TestEverySettingHasOneWriter pins the ownership table to the struct: a new
// field must be classified, and a listed name must be a real field.
func TestEverySettingHasOneWriter(t *testing.T) {
	typ := reflect.TypeOf(Settings{})
	listed := map[string]int{}
	for _, list := range [][]string{appOwnedFields, engineOwnedFields, storeOwnedFields} {
		for _, name := range list {
			listed[name]++
		}
	}
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if field.PkgPath != "" {
			continue
		}
		switch listed[field.Name] {
		case 1:
			delete(listed, field.Name)
		case 0:
			t.Errorf("%s is in no ownership list", field.Name)
		default:
			t.Errorf("%s is in %d ownership lists", field.Name, listed[field.Name])
			delete(listed, field.Name)
		}
	}
	for name := range listed {
		t.Errorf("%s is not a Settings field", name)
	}
}

func TestMergeCopiesOnlyOwnedFields(t *testing.T) {
	src := Default()
	src.Beta = true
	src.Volume = 42
	dst := Default()
	dst.Beta = false
	dst.Volume = 80

	MergeApp(&dst, &src)
	if !dst.Beta {
		t.Fatal("MergeApp did not copy the app-owned Beta")
	}
	if dst.Volume != 80 {
		t.Fatalf("MergeApp copied the engine-owned Volume: %v", dst.Volume)
	}

	dst = Default()
	MergeEngine(&dst, &src)
	if dst.Volume != 42 {
		t.Fatalf("MergeEngine did not copy the engine-owned Volume: %v", dst.Volume)
	}
	if dst.Beta {
		t.Fatal("MergeEngine copied the app-owned Beta")
	}
}
