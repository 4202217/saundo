package main

import (
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCatalogPassed(t *testing.T) {
	run := []byte("=== RUN   TestCatalog\n--- PASS: TestCatalog (0.00s)\nPASS\nok  \tsaundo/internal/app\t0.031s\n")
	if !catalogPassed(run) {
		t.Fatal("catalogPassed rejected a real pass line")
	}
	none := []byte("ok  \tsaundo/internal/app\t0.035s [no tests to run]\n")
	if catalogPassed(none) {
		t.Fatal("catalogPassed accepted a run with no matching test")
	}
	other := []byte("--- PASS: TestCatalogSomething (0.00s)\n")
	if catalogPassed(other) {
		t.Fatal("catalogPassed accepted another test's pass line")
	}
}

func writeTestPNG(t *testing.T, path string, c color.RGBA) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 3))
	img.Set(1, 1, c)
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func recordAll(t *testing.T, dir string, names ...string) []fileRecord {
	t.Helper()
	var files []fileRecord
	for _, name := range names {
		rec, err := recordFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, rec)
	}
	return files
}

func TestRecordFileHashesAndSizes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.png")
	writeTestPNG(t, path, color.RGBA{R: 255, A: 255})
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

func TestRebaselineKeepsFileSetUpdatesHashesAndCopiesPNGs(t *testing.T) {
	dir := t.TempDir()
	baseDir := filepath.Join(dir, "base")
	againstDir := filepath.Join(dir, "against")
	outDir := filepath.Join(dir, "out")
	for _, d := range []string{baseDir, againstDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeTestPNG(t, filepath.Join(baseDir, "a.png"), color.RGBA{R: 10, A: 255})
	writeTestPNG(t, filepath.Join(baseDir, "b.png"), color.RGBA{R: 10, A: 255})
	writeTestPNG(t, filepath.Join(againstDir, "a.png"), color.RGBA{R: 10, A: 255})
	writeTestPNG(t, filepath.Join(againstDir, "b.png"), color.RGBA{R: 20, A: 255})
	base := manifest{Renderer: "r", Files: recordAll(t, baseDir, "a.png", "b.png")}
	against := manifest{Renderer: "r", Files: recordAll(t, againstDir, "a.png", "b.png")}
	basePath := filepath.Join(baseDir, "manifest.json")
	againstPath := filepath.Join(againstDir, "manifest.json")
	outPath := filepath.Join(outDir, "manifest.json")
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
	if len(got.Files) != 2 || got.Files[1].Name != "b.png" || got.Files[1].SHA256 != against.Files[1].SHA256 {
		t.Fatalf("rebaseline = %+v, want a.png and updated b.png only", got.Files)
	}
	for _, name := range []string{"a.png", "b.png"} {
		rec, err := recordFile(filepath.Join(outDir, name))
		if err != nil {
			t.Fatalf("%s was not copied: %v", name, err)
		}
		var want fileRecord
		for _, f := range got.Files {
			if f.Name == name {
				want = f
			}
		}
		if rec != want {
			t.Fatalf("%s = %+v, want the manifest record %+v", name, rec, want)
		}
	}
}

func TestSubsetSelectsStemsAndCopiesPNGs(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "render")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestPNG(t, filepath.Join(src, "a-en.png"), color.RGBA{R: 10, A: 255})
	writeTestPNG(t, filepath.Join(src, "a-zh.png"), color.RGBA{R: 20, A: 255})
	writeTestPNG(t, filepath.Join(src, "b-en.png"), color.RGBA{R: 30, A: 255})
	writeTestPNG(t, filepath.Join(src, "c-en.png"), color.RGBA{R: 40, A: 255})
	full := manifest{Commit: "abc", Dirty: "dirty", Renderer: "r", Files: recordAll(t, src, "a-en.png", "a-zh.png", "b-en.png", "c-en.png")}
	basePath := filepath.Join(src, "manifest.json")
	if err := writeManifest(basePath, full); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "baseline")
	if err := runSubset([]string{"-baseline", basePath, "-states", "a,b", "-out", out}); err != nil {
		t.Fatal(err)
	}
	got, err := readManifest(filepath.Join(out, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"a-en.png", "a-zh.png", "b-en.png"}
	if got.Renderer != "r" || got.Commit != "abc" {
		t.Fatalf("subset manifest = %+v, want renderer r and commit abc", got)
	}
	if len(got.Files) != len(want) {
		t.Fatalf("subset kept %d files, want %d", len(got.Files), len(want))
	}
	for i, name := range want {
		if got.Files[i].Name != name {
			t.Fatalf("subset file %d = %s, want %s", i, got.Files[i].Name, name)
		}
		rec, err := recordFile(filepath.Join(out, name))
		if err != nil {
			t.Fatalf("%s was not copied: %v", name, err)
		}
		if rec != got.Files[i] {
			t.Fatalf("%s = %+v, want the manifest record %+v", name, rec, got.Files[i])
		}
	}
	if _, err := os.Stat(filepath.Join(out, "c-en.png")); !os.IsNotExist(err) {
		t.Fatal("subset copied c-en.png, want only the selected stems")
	}
}

func TestSubsetRefusesAStemWithNoFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.json")
	if err := writeManifest(path, manifest{Renderer: "r", Files: []fileRecord{{Name: "a-en.png"}}}); err != nil {
		t.Fatal(err)
	}
	err := runSubset([]string{"-baseline", path, "-states", "a,missing", "-out", t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "missing matches no file") {
		t.Fatalf("subset = %v, want missing to match no file", err)
	}
}

func TestSubsetRefusesASourceThatDisagreesWithTheManifest(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "render")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(src, "a-en.png")
	writeTestPNG(t, path, color.RGBA{R: 10, A: 255})
	full := manifest{Renderer: "r", Files: recordAll(t, src, "a-en.png")}
	basePath := filepath.Join(src, "manifest.json")
	if err := writeManifest(basePath, full); err != nil {
		t.Fatal(err)
	}
	writeTestPNG(t, path, color.RGBA{R: 20, A: 255})
	out := filepath.Join(dir, "baseline")
	err := runSubset([]string{"-baseline", basePath, "-states", "a", "-out", out})
	if !errors.Is(err, errInconclusive) {
		t.Fatalf("subset = %v, want an inconclusive error", err)
	}
	if !strings.Contains(err.Error(), "a-en.png does not match the stable manifest") {
		t.Fatalf("subset = %v, want the changed name", err)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("subset touched the output before the source proved valid")
	}
}

func TestDiffVerifiesTheBaselinePNGs(t *testing.T) {
	dir := t.TempDir()
	baseDir := filepath.Join(dir, "baseline")
	againstDir := filepath.Join(dir, "against")
	for _, d := range []string{baseDir, againstDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeTestPNG(t, filepath.Join(baseDir, "a-en.png"), color.RGBA{R: 10, A: 255})
	writeTestPNG(t, filepath.Join(againstDir, "a-en.png"), color.RGBA{R: 10, A: 255})
	basePath := filepath.Join(baseDir, "manifest.json")
	againstPath := filepath.Join(againstDir, "manifest.json")
	if err := writeManifest(basePath, manifest{Renderer: "r", Files: recordAll(t, baseDir, "a-en.png")}); err != nil {
		t.Fatal(err)
	}
	if err := writeManifest(againstPath, manifest{Renderer: "r", Files: recordAll(t, againstDir, "a-en.png")}); err != nil {
		t.Fatal(err)
	}
	if err := runDiff([]string{"-baseline", basePath, "-against", againstPath}); err != nil {
		t.Fatalf("unchanged diff = %v, want a match", err)
	}
	writeTestPNG(t, filepath.Join(baseDir, "a-en.png"), color.RGBA{R: 20, A: 255})
	err := runDiff([]string{"-baseline", basePath, "-against", againstPath})
	if !errors.Is(err, errInconclusive) {
		t.Fatalf("diff = %v, want an inconclusive error", err)
	}
	if !strings.Contains(err.Error(), "changed a-en.png") {
		t.Fatalf("diff = %v, want the changed name", err)
	}
}

func TestDiffRefusesAMissingBaseline(t *testing.T) {
	dir := t.TempDir()
	writeTestPNG(t, filepath.Join(dir, "a-en.png"), color.RGBA{R: 10, A: 255})
	againstPath := filepath.Join(dir, "manifest.json")
	if err := writeManifest(againstPath, manifest{Renderer: "r", Files: recordAll(t, dir, "a-en.png")}); err != nil {
		t.Fatal(err)
	}
	err := runDiff([]string{"-baseline", filepath.Join(dir, "absent.json"), "-against", againstPath})
	if !errors.Is(err, errInconclusive) {
		t.Fatalf("diff = %v, want an inconclusive error", err)
	}
}

func TestDiffRefusesARendererMismatch(t *testing.T) {
	dir := t.TempDir()
	writeTestPNG(t, filepath.Join(dir, "a-en.png"), color.RGBA{R: 10, A: 255})
	files := recordAll(t, dir, "a-en.png")
	basePath := filepath.Join(dir, "base.json")
	againstPath := filepath.Join(dir, "against.json")
	if err := writeManifest(basePath, manifest{Renderer: "raster", Files: files}); err != nil {
		t.Fatal(err)
	}
	if err := writeManifest(againstPath, manifest{Renderer: "metal", Files: files}); err != nil {
		t.Fatal(err)
	}
	err := runDiff([]string{"-baseline", basePath, "-against", againstPath})
	if !errors.Is(err, errInconclusive) {
		t.Fatalf("diff = %v, want an inconclusive error", err)
	}
	if !strings.Contains(err.Error(), "renderer differs") {
		t.Fatalf("diff = %v, want the renderer mismatch", err)
	}
}

func TestReadManifestRejectsNoRenderer(t *testing.T) {
	path := filepath.Join(t.TempDir(), "manifest.json")
	if err := os.WriteFile(path, []byte(`{"files":[{"name":"a.png","sha256":"1","width":1,"height":1}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readManifest(path); err == nil {
		t.Fatal("readManifest accepted a manifest with no renderer")
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
