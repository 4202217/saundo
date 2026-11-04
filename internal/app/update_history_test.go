package app

import (
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"saundo/cmd/saundo/locales"
	"saundo/internal/settings"
)

func TestChangelogURLLocale(t *testing.T) {
	cases := map[string]string{
		"":        updateChangelog,
		"en":      updateChangelog,
		"zh-CN":   updateChangelogZH,
		"zh-Hant": updateChangelogZH,
	}
	for locale, want := range cases {
		if got := changelogURL(locale); got != want {
			t.Errorf("changelogURL(%q) = %q, want %q", locale, got, want)
		}
	}
}

// The manifest's notes are English by contract; the fetched changelog is the
// localized one, so its entry for the offered version replaces them.
func TestNotesEntriesUseTheLocalizedChangelog(t *testing.T) {
	a := &App{notesRange: true}
	a.upd = updateState{version: "0.4.2", notes: "### Added\n\n- English entry\n"}
	a.history = []releaseNote{
		{Version: "0.4.2", Notes: "### 新增\n\n- 中文条目\n"},
		{Version: "0.4.1", Notes: "### 修复\n\n- 旧条目\n"},
	}
	entries := a.notesEntries()
	if len(entries) != 2 {
		t.Fatalf("entries = %d", len(entries))
	}
	if !strings.Contains(entries[0].Notes, "中文条目") {
		t.Fatalf("offered notes = %q, want the fetched Chinese entry", entries[0].Notes)
	}
	if !strings.Contains(entries[1].Notes, "旧条目") {
		t.Fatalf("history notes = %q", entries[1].Notes)
	}

	a.notesRange = false
	if entries = a.notesEntries(); len(entries) != 1 || !strings.Contains(entries[0].Notes, "中文条目") {
		t.Fatalf("latest-only notes = %+v", entries)
	}
}

// A failed or empty fetch keeps the manifest's notes.
func TestNotesEntriesFallBackToTheManifest(t *testing.T) {
	a := &App{notesRange: true}
	a.upd = updateState{version: "0.4.2", notes: "### Added\n\n- English entry\n"}
	a.historyState = historyFailed
	entries := a.notesEntries()
	if len(entries) != 1 || !strings.Contains(entries[0].Notes, "English entry") {
		t.Fatalf("failed fetch notes = %+v", entries)
	}

	a.history = []releaseNote{{Version: "0.4.2"}}
	if entries = a.notesEntries(); !strings.Contains(entries[0].Notes, "English entry") {
		t.Fatalf("empty fetched entry replaced the manifest notes: %q", entries[0].Notes)
	}
}

// The history is fetched in the app language, so a language switch must not
// reuse the old fetch.
func TestLanguageSwitchRefetchesHistory(t *testing.T) {
	a := previewApp()
	a.upd = updateState{version: "0.4.2", notes: "English", up: release()}
	a.async = func(func()) {}
	locale := i18n.Locale()

	a.historyKey, a.historyLocale = "0.4.2", locale
	a.historyState = historyReady
	a.history = []releaseNote{{Version: "0.4.2", Notes: "English"}}
	a.loadReleaseHistory()
	if a.historyState != historyReady {
		t.Fatal("the history was fetched again in the same language")
	}

	a.historyLocale = locale + "-other"
	a.loadReleaseHistory()
	if a.historyState != historyLoading || a.history != nil {
		t.Fatalf("a language switch kept the old history: state %d, %d entries", a.historyState, len(a.history))
	}
}

// The dialog is the surface the report showed: the Chinese UI must not fall
// back to the English manifest notes.
func TestNotesDialogRendersTheLocalizedEntry(t *testing.T) {
	a := previewApp()
	a.cfg = settings.Default()
	a.page = pageUpdate
	a.upd = updateState{
		version: "0.4.2",
		date:    time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC),
		notes:   "### Added\n\n- English entry\n",
	}
	a.history = []releaseNote{{Version: "0.4.2", Notes: "### 新增\n\n- 中文条目\n"}}
	a.historyState = historyReady
	a.notesOpen = true
	a.notesRange = true
	tt := ui.NewTester(a.settingsView, 720, 500)
	if tt.HasText("English entry") {
		t.Fatalf("the dialog rendered the English manifest notes; texts: %q", tt.Texts())
	}
	if !tt.HasText("中文条目") {
		t.Fatalf("the dialog is missing the localized entry; texts: %q", tt.Texts())
	}
}

// While the localized fetch is in flight, the dialog must not show the
// manifest's English notes first.
func TestNotesDialogHidesTheEnglishNotesWhileLoading(t *testing.T) {
	a := previewApp()
	a.cfg = settings.Default()
	a.page = pageUpdate
	a.upd = updateState{version: "0.4.2", notes: "### Added\n\n- English entry\n"}
	a.historyState = historyLoading
	a.notesOpen = true
	a.notesRange = true
	tt := ui.NewTester(a.settingsView, 720, 500)
	if tt.HasText("English entry") {
		t.Fatalf("the dialog flashed the English notes while loading; texts: %q", tt.Texts())
	}
	if !tt.HasText(locales.UpdateHistoryLoading()) {
		t.Fatalf("the dialog has no loading state; texts: %q", tt.Texts())
	}
}

func TestParseChangelog(t *testing.T) {
	text := "# Changelog\n\nAll notable changes.\n\n" +
		"## 0.4.2 - 2026-11-08\n\n### Changed\n\n- b\n\n" +
		"## 0.4.1 - 2026-11-02\n\n### Fixed\n\n- a\n"
	notes := parseChangelog(text)
	if len(notes) != 2 {
		t.Fatalf("sections = %d", len(notes))
	}
	if notes[0].Version != "0.4.2" || notes[0].Date != time.Date(2026, 11, 8, 0, 0, 0, 0, time.UTC) {
		t.Fatalf("first = %+v", notes[0])
	}
	if !strings.Contains(notes[0].Notes, "### Changed") || !strings.Contains(notes[0].Notes, "- b") {
		t.Fatalf("first notes = %q", notes[0].Notes)
	}
	if notes[1].Version != "0.4.1" || !strings.Contains(notes[1].Notes, "- a") {
		t.Fatalf("second = %+v", notes[1])
	}
}

func TestFilterNotes(t *testing.T) {
	notes := []releaseNote{
		{Version: "0.4.4"},
		{Version: "0.4.3"},
		{Version: "0.4.2"},
		{Version: "0.4.1"},
		{Version: "0.4.0"},
		{Version: "nightly"},
	}
	versions := func(rs []releaseNote) string {
		out := make([]string, len(rs))
		for i, r := range rs {
			out[i] = r.Version
		}
		return strings.Join(out, ",")
	}

	if got := versions(filterNotes("0.4.0", "0.4.3", notes)); got != "0.4.3,0.4.2,0.4.1" {
		t.Fatalf("from 0.4.0: %s", got)
	}
	if got := versions(filterNotes("0.4.2", "0.4.3", notes)); got != "0.4.3" {
		t.Fatalf("from 0.4.2: %s", got)
	}
	// A build without a semantic version sees everything up to the offered
	// one.
	if got := versions(filterNotes("dev", "0.4.3", notes)); got != "0.4.3,0.4.2,0.4.1,0.4.0" {
		t.Fatalf("from dev: %s", got)
	}
}

func TestSummarizeNotes(t *testing.T) {
	cases := []struct {
		name  string
		notes string
		kind  string
		n     int
	}{
		{
			"fixed",
			"### Fixed\n\n- a\n- b\n- c\n",
			"fixed", 3,
		},
		{
			"mixed takes the largest",
			"### Added\n\n- a\n- b\n\n### Fixed\n\n- x\n",
			"added", 2,
		},
		{
			"no headings counts bullets",
			"- one\n- two\n",
			"updated", 2,
		},
	}
	for _, tc := range cases {
		kind, n := summarizeNotes(tc.notes)
		if kind != tc.kind || n != tc.n {
			t.Errorf("%s: summarizeNotes = %q, %d; want %q, %d", tc.name, kind, n, tc.kind, tc.n)
		}
	}
}
