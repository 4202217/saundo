package app

import (
	"strings"
	"testing"
	"time"
)

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
