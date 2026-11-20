package receivers

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestChannelPatchAndPersistence(t *testing.T) {
	dir := t.TempDir()
	store := NewStoreAt(filepath.Join(dir, "receivers.json"))
	left := ChannelLeft
	entry := Entry{Key: "AA:01", Kind: KindDiscovered, Name: "客厅 HomePod"}
	entry = Patch{Channel: &left}.Apply(entry)
	if entry.Channel != ChannelLeft {
		t.Fatalf("patched channel = %q, want left", entry.Channel)
	}
	reg := Registry{Version: SchemaVersion, Entries: []Entry{entry}}
	if err := store.Save(reg); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	got, ok := loaded.Get("AA:01")
	if !ok || got.Channel != ChannelLeft {
		t.Fatalf("persisted channel = %q ok=%v, want left", got.Channel, ok)
	}

	// An unknown or empty patch value reads as stereo.
	for _, value := range []string{"", "surround", "LEFT"} {
		patch := value
		applied := Patch{Channel: &patch}.Apply(entry)
		if applied.Channel != ChannelStereo {
			t.Errorf("patch %q = %q, want stereo", value, applied.Channel)
		}
	}
}

func TestViewCarriesChannel(t *testing.T) {
	target := Target{
		Entry:  Entry{Key: "AA:01", Kind: KindDiscovered, Channel: ChannelRight},
		Device: discovered("AA:01", "客厅 HomePod", "192.0.2.10", 7000),
	}
	if got := target.View(false).Channel; got != ChannelRight {
		t.Fatalf("view channel = %q, want right", got)
	}
	target.Entry.Channel = "surround"
	if got := target.View(false).Channel; got != ChannelStereo {
		t.Fatalf("unknown view channel = %q, want stereo", got)
	}
}

// TestVersionOneFileSurvivesTheSchemaBump pins the upgrade guard: a file
// written before the channel and the combo rows loads its fields, is restamped
// on save, and comes back whole.
func TestVersionOneFileSurvivesTheSchemaBump(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "receivers.json")
	seed := `{"version":1,"entries":[
		{"key":"AA:01","kind":"discovered","name":"客厅","channel":"left"},
		{"key":"combo:01","kind":"combo","name":"HomePods","members":[{"id":"AA:01"},{"id":"AA:02"}]}
	]}`
	if err := os.WriteFile(path, []byte(seed), 0o600); err != nil {
		t.Fatal(err)
	}
	store := NewStoreAt(path)
	reg, err := store.Load()
	if err != nil {
		t.Fatalf("a v1 file was refused: %v", err)
	}
	one, ok := reg.Get("AA:01")
	if !ok || one.Channel != ChannelLeft {
		t.Fatalf("channel lost: %+v", one)
	}
	combo, ok := reg.Get("combo:01")
	if !ok || !combo.IsCombo() || len(combo.Members) != 2 {
		t.Fatalf("combo row lost: %+v", combo)
	}
	if err := store.Save(reg); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var saved Registry
	if err := json.Unmarshal(raw, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Version != SchemaVersion {
		t.Fatalf("saved version = %d, want %d", saved.Version, SchemaVersion)
	}
	if one, _ := saved.Get("AA:01"); one.Channel != ChannelLeft {
		t.Fatal("the restamped file dropped the channel")
	}
}
