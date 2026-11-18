package receivers

import (
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
