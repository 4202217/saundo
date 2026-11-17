package main

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestRecordFileHashesAndSizes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.png")
	img := image.NewRGBA(image.Rect(0, 0, 4, 3))
	img.Set(1, 1, color.RGBA{R: 255, A: 255})
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	rec, err := recordFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Name != "state.png" || rec.Width != 4 || rec.Height != 3 {
		t.Fatalf("record = %+v, want state.png 4x3", rec)
	}
	if len(rec.SHA256) != 64 {
		t.Fatalf("sha256 = %q, want 64 hex characters", rec.SHA256)
	}
}

func TestCompareManifests(t *testing.T) {
	base := manifest{Files: []fileRecord{
		{Name: "a.png", SHA256: "1", Width: 10, Height: 20},
		{Name: "b.png", SHA256: "2", Width: 10, Height: 20},
		{Name: "c.png", SHA256: "3", Width: 10, Height: 20},
	}}
	against := manifest{Files: []fileRecord{
		{Name: "a.png", SHA256: "1", Width: 10, Height: 20},
		{Name: "b.png", SHA256: "9", Width: 10, Height: 20},
		{Name: "d.png", SHA256: "4", Width: 10, Height: 20},
	}}
	problems, extras := compareManifests(base, against)
	wantProblems := []string{"changed b.png", "missing c.png"}
	wantExtras := []string{"d.png"}
	if len(problems) != len(wantProblems) {
		t.Fatalf("problems = %q, want %q", problems, wantProblems)
	}
	for i := range wantProblems {
		if problems[i] != wantProblems[i] {
			t.Fatalf("problems = %q, want %q", problems, wantProblems)
		}
	}
	if len(extras) != len(wantExtras) || extras[0] != wantExtras[0] {
		t.Fatalf("extras = %q, want %q", extras, wantExtras)
	}
}

func TestCompareManifestsReportsSizeChange(t *testing.T) {
	base := manifest{Files: []fileRecord{{Name: "a.png", SHA256: "1", Width: 10, Height: 20}}}
	against := manifest{Files: []fileRecord{{Name: "a.png", SHA256: "1", Width: 10, Height: 21}}}
	problems, _ := compareManifests(base, against)
	if len(problems) != 1 || problems[0] != "size a.png, baseline 10x20 against 10x21" {
		t.Fatalf("problems = %q", problems)
	}
}

func TestBaselineKeepsOnlyMatchingFiles(t *testing.T) {
	dir := t.TempDir()
	aPath := filepath.Join(dir, "a.json")
	bPath := filepath.Join(dir, "b.json")
	outPath := filepath.Join(dir, "stable.json")
	a := manifest{Renderer: "r", Files: []fileRecord{
		{Name: "a.png", SHA256: "1", Width: 2, Height: 2},
		{Name: "b.png", SHA256: "2", Width: 2, Height: 2},
	}}
	b := manifest{Renderer: "r", Files: []fileRecord{
		{Name: "a.png", SHA256: "1", Width: 2, Height: 2},
		{Name: "b.png", SHA256: "9", Width: 2, Height: 2},
	}}
	if err := writeManifest(aPath, a); err != nil {
		t.Fatal(err)
	}
	if err := writeManifest(bPath, b); err != nil {
		t.Fatal(err)
	}
	if err := runBaseline([]string{"-a", aPath, "-b", bPath, "-out", outPath}); err != nil {
		t.Fatal(err)
	}
	stable, err := readManifest(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(stable.Files) != 1 || stable.Files[0].Name != "a.png" {
		t.Fatalf("stable = %+v, want only a.png", stable.Files)
	}
}

func TestReadManifestRejectsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.json")
	if err := os.WriteFile(path, []byte(`{"files":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readManifest(path); err == nil {
		t.Fatal("readManifest accepted an empty file list")
	}
}

func TestRebaselineKeepsFileSetAndUpdatesHashes(t *testing.T) {
	dir := t.TempDir()
	basePath := filepath.Join(dir, "base.json")
	againstPath := filepath.Join(dir, "against.json")
	outPath := filepath.Join(dir, "out.json")
	base := manifest{Renderer: "r", Files: []fileRecord{
		{Name: "a.png", SHA256: "1", Width: 2, Height: 2},
		{Name: "b.png", SHA256: "2", Width: 2, Height: 2},
	}}
	against := manifest{Renderer: "r", Files: []fileRecord{
		{Name: "a.png", SHA256: "1", Width: 2, Height: 2},
		{Name: "b.png", SHA256: "9", Width: 2, Height: 2},
		{Name: "c.png", SHA256: "3", Width: 2, Height: 2},
	}}
	if err := writeManifest(basePath, base); err != nil {
		t.Fatal(err)
	}
	if err := writeManifest(againstPath, against); err != nil {
		t.Fatal(err)
	}
	if err := runRebaseline([]string{"-baseline", basePath, "-against", againstPath, "-out", outPath}); err != nil {
		t.Fatal(err)
	}
	got, err := readManifest(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Files) != 2 || got.Files[1].Name != "b.png" || got.Files[1].SHA256 != "9" {
		t.Fatalf("rebaseline = %+v, want a.png and updated b.png only", got.Files)
	}
}

func TestCleanOutputKeepsForeignFiles(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"stale.png", "manifest.json", "keep.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := cleanOutput(dir); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"stale.png", "manifest.json"} {
		if _, err := os.Stat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Fatalf("%s survived cleanOutput", name)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "keep.txt")); err != nil {
		t.Fatalf("cleanOutput removed a foreign file: %v", err)
	}
}

func TestManifestRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.json")
	want := manifest{
		Commit:   "abc",
		Dirty:    "clean",
		Renderer: "ui.Tester/raster",
		Files:    []fileRecord{{Name: "a.png", SHA256: "1", Width: 2, Height: 3}},
	}
	if err := writeManifest(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := readManifest(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Commit != want.Commit || got.Renderer != want.Renderer || len(got.Files) != 1 || got.Files[0].Name != "a.png" {
		t.Fatalf("round trip = %+v", got)
	}
}
