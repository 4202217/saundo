// Command check-ui renders the UI state matrix and compares the result with a
// baseline manifest.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"image"
	_ "image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// errInconclusive marks a comparison that could not decide. main exits with 2
// so a missing manifest never reads as a passing diff.
var errInconclusive = errors.New("inconclusive")

type manifest struct {
	Commit    string       `json:"commit"`
	Dirty     string       `json:"dirty"`
	Renderer  string       `json:"renderer"`
	Generated string       `json:"generated"`
	Files     []fileRecord `json:"files"`
}

type fileRecord struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

func main() {
	root, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "check-ui:", err)
		os.Exit(2)
	}
	if err := run(root, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "check-ui:", err)
		if errors.Is(err, errInconclusive) {
			os.Exit(2)
		}
		os.Exit(1)
	}
}

func run(root string, args []string) error {
	if len(args) == 0 {
		return usage()
	}
	switch args[0] {
	case "render":
		return runRender(root, args[1:])
	case "catalog":
		return runCatalog(root)
	case "wasm":
		return runWasm(root)
	case "baseline":
		return runBaseline(args[1:])
	case "rebaseline":
		return runRebaseline(args[1:])
	case "diff":
		return runDiff(args[1:])
	default:
		return usage()
	}
}

func usage() error {
	return fmt.Errorf("usage: check-ui render -out <dir> [-only <prefixes>] [-renderer <name>] | check-ui catalog | check-ui wasm | check-ui baseline -a <manifest> -b <manifest> [-out <manifest>] | check-ui rebaseline -baseline <manifest> -against <manifest> [-out <manifest>] | check-ui diff -baseline <manifest> -against <manifest> [-strict]")
}

// runWasm builds the browser preview with the preview tag into a temp file.
// It proves the harness compiles for js/wasm and keeps the result out of any
// shipped path.
func runWasm(root string) error {
	f, err := os.CreateTemp("", "saundo-preview-*.wasm")
	if err != nil {
		return err
	}
	path := f.Name()
	if err := f.Close(); err != nil {
		os.Remove(path)
		return err
	}
	defer os.Remove(path)
	cmd := exec.Command("go", "build", "-tags", "preview", "-o", path, "./cmd/saundo/preview")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm")
	start := time.Now()
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("wasm: %w\n%s", err, output)
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	fmt.Printf("check-ui: wasm built, %d bytes in %s\n", info.Size(), time.Since(start).Round(time.Millisecond))
	return nil
}

// runCatalog validates the scenario table through its test. The catalog
// carries the preview tag, so the check runs under that tag. A run that finds
// no matching test exits 0, so the pass line is what proves the test ran.
func runCatalog(root string) error {
	cmd := exec.Command("go", "test", "-tags", "preview", "./internal/app", "-run", "^TestCatalog$", "-count=1", "-v")
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("catalog: %w\n%s", err, output)
	}
	if !catalogPassed(output) {
		return fmt.Errorf("catalog: TestCatalog did not run\n%s", output)
	}
	fmt.Printf("%s", output)
	fmt.Println("check-ui: catalog ok")
	return nil
}

// catalogPassed reports whether the test output carries TestCatalog's own
// pass line. The trailing space keeps TestCatalogFoo from matching.
func catalogPassed(output []byte) bool {
	return bytes.Contains(output, []byte("--- PASS: TestCatalog "))
}

func runRender(root string, args []string) error {
	fs := flag.NewFlagSet("render", flag.ContinueOnError)
	out := fs.String("out", "", "output directory")
	only := fs.String("only", "", "comma-separated state prefixes")
	renderer := fs.String("renderer", "ui.Tester/raster", "renderer label recorded in the manifest")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *out == "" {
		return fmt.Errorf("render: -out is required")
	}
	dir, err := filepath.Abs(*out)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := cleanOutput(dir); err != nil {
		return err
	}
	cmd := exec.Command("go", "test", "-tags", "preview", "./internal/app", "-run", "TestRenderPreviews", "-count=1")
	cmd.Dir = root
	env := append(os.Environ(), "SAUNDO_PREVIEW_DIR="+dir)
	if *only != "" {
		env = append(env, "SAUNDO_PREVIEW_ONLY="+*only)
	}
	cmd.Env = env
	start := time.Now()
	output, err := cmd.CombinedOutput()
	elapsed := time.Since(start)
	if err != nil {
		return fmt.Errorf("render: matrix failed after %s: %w\n%s", elapsed.Round(time.Millisecond), err, output)
	}
	files, err := collect(dir)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("%w: the matrix wrote no PNG files to %s", errInconclusive, dir)
	}
	m := manifest{
		Commit:    git(root, "rev-parse", "HEAD"),
		Dirty:     dirty(root),
		Renderer:  *renderer,
		Generated: time.Now().UTC().Format(time.RFC3339),
		Files:     files,
	}
	if err := writeManifest(filepath.Join(dir, "manifest.json"), m); err != nil {
		return err
	}
	fmt.Printf("check-ui: %d files in %s, matrix took %s, renderer %s\n", len(files), dir, elapsed.Round(time.Millisecond), *renderer)
	return nil
}

// cleanOutput removes the artifacts of an earlier render. The matrix writes
// only the wanted states, so a reused directory would otherwise keep a
// deleted or filtered-out state alive in the next manifest.
func cleanOutput(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".png") && name != "manifest.json" {
			continue
		}
		if err := os.Remove(filepath.Join(dir, name)); err != nil {
			return err
		}
	}
	return nil
}

func collect(dir string) ([]fileRecord, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []fileRecord
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".png") {
			continue
		}
		rec, err := recordFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		files = append(files, rec)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })
	return files, nil
}

func recordFile(path string) (fileRecord, error) {
	f, err := os.Open(path)
	if err != nil {
		return fileRecord{}, err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fileRecord{}, err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return fileRecord{}, err
	}
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return fileRecord{}, fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	return fileRecord{
		Name:   filepath.Base(path),
		SHA256: fmt.Sprintf("%x", h.Sum(nil)),
		Width:  cfg.Width,
		Height: cfg.Height,
	}, nil
}

// runBaseline writes a manifest of the files whose bytes match between two
// renders. The matrix starts its clock at time.Now, so animated states settle
// in a different phase on each run. The stable set is the only honest pixel
// baseline, and every excluded name is printed for the record.
func runBaseline(args []string) error {
	fs := flag.NewFlagSet("baseline", flag.ContinueOnError)
	aPath := fs.String("a", "", "first manifest")
	bPath := fs.String("b", "", "second manifest")
	out := fs.String("out", "", "stable manifest output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *aPath == "" || *bPath == "" {
		return fmt.Errorf("baseline: -a and -b are required")
	}
	a, err := readManifest(*aPath)
	if err != nil {
		return fmt.Errorf("%w: a: %v", errInconclusive, err)
	}
	b, err := readManifest(*bPath)
	if err != nil {
		return fmt.Errorf("%w: b: %v", errInconclusive, err)
	}
	if a.Renderer != b.Renderer {
		return fmt.Errorf("%w: renderer differs, a %q b %q", errInconclusive, a.Renderer, b.Renderer)
	}
	bFiles := map[string]fileRecord{}
	for _, f := range b.Files {
		bFiles[f.Name] = f
	}
	stable := manifest{
		Commit:    a.Commit,
		Dirty:     a.Dirty,
		Renderer:  a.Renderer,
		Generated: time.Now().UTC().Format(time.RFC3339),
	}
	var unstable []string
	for _, f := range a.Files {
		g, ok := bFiles[f.Name]
		if ok && g.SHA256 == f.SHA256 && g.Width == f.Width && g.Height == f.Height {
			stable.Files = append(stable.Files, f)
			continue
		}
		unstable = append(unstable, f.Name)
	}
	if len(stable.Files) == 0 {
		return fmt.Errorf("%w: the two manifests share no stable file", errInconclusive)
	}
	if *out == "" {
		*out = filepath.Join(filepath.Dir(*aPath), "manifest-stable.json")
	}
	if err := writeManifest(*out, stable); err != nil {
		return err
	}
	fmt.Printf("check-ui: %d stable files, %d unstable\n", len(stable.Files), len(unstable))
	for _, name := range unstable {
		fmt.Println("unstable", name)
	}
	return nil
}

// runRebaseline updates the hashes of the existing baseline file set from a
// new render. It never adds or removes a state: a missing file fails, and an
// extra file is ignored. That keeps the baseline membership fixed while an
// intentional visual change updates only the states under review.
func runRebaseline(args []string) error {
	fs := flag.NewFlagSet("rebaseline", flag.ContinueOnError)
	baselinePath := fs.String("baseline", "", "baseline manifest whose file set is kept")
	againstPath := fs.String("against", "", "manifest with the new hashes")
	out := fs.String("out", "", "updated manifest output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *baselinePath == "" || *againstPath == "" {
		return fmt.Errorf("rebaseline: -baseline and -against are required")
	}
	base, err := readManifest(*baselinePath)
	if err != nil {
		return fmt.Errorf("%w: baseline: %v", errInconclusive, err)
	}
	against, err := readManifest(*againstPath)
	if err != nil {
		return fmt.Errorf("%w: against: %v", errInconclusive, err)
	}
	if base.Renderer != against.Renderer {
		return fmt.Errorf("%w: renderer differs, baseline %q against %q", errInconclusive, base.Renderer, against.Renderer)
	}
	next := map[string]fileRecord{}
	for _, f := range against.Files {
		next[f.Name] = f
	}
	changed := 0
	for i, f := range base.Files {
		g, ok := next[f.Name]
		if !ok {
			return fmt.Errorf("rebaseline: %s is missing from the new manifest", f.Name)
		}
		if f.SHA256 == g.SHA256 && f.Width == g.Width && f.Height == g.Height {
			continue
		}
		base.Files[i] = g
		changed++
		fmt.Println("updated", f.Name)
	}
	base.Commit = against.Commit
	base.Dirty = against.Dirty
	base.Generated = time.Now().UTC().Format(time.RFC3339)
	outPath := *out
	if outPath == "" {
		outPath = *baselinePath
	}
	if err := writeManifest(outPath, base); err != nil {
		return err
	}
	fmt.Printf("check-ui: updated %d of %d files\n", changed, len(base.Files))
	return nil
}

func runDiff(args []string) error {
	fs := flag.NewFlagSet("diff", flag.ContinueOnError)
	baselinePath := fs.String("baseline", "", "baseline manifest")
	againstPath := fs.String("against", "", "manifest to compare")
	strict := fs.Bool("strict", false, "fail on files that are not in the baseline")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *baselinePath == "" || *againstPath == "" {
		return fmt.Errorf("diff: -baseline and -against are required")
	}
	baseline, err := readManifest(*baselinePath)
	if err != nil {
		return fmt.Errorf("%w: baseline: %v", errInconclusive, err)
	}
	against, err := readManifest(*againstPath)
	if err != nil {
		return fmt.Errorf("%w: against: %v", errInconclusive, err)
	}
	if baseline.Renderer != against.Renderer {
		return fmt.Errorf("%w: renderer differs, baseline %q against %q", errInconclusive, baseline.Renderer, against.Renderer)
	}
	problems, extras := compareManifests(baseline, against)
	if *strict {
		for _, name := range extras {
			problems = append(problems, "extra "+name)
		}
		sort.Strings(problems)
	}
	if len(problems) > 0 {
		return fmt.Errorf("diff: %d difference(s)\n%s", len(problems), strings.Join(problems, "\n"))
	}
	if len(extras) > 0 {
		fmt.Printf("check-ui: %d files not in the baseline were not compared\n", len(extras))
	}
	fmt.Printf("check-ui: %d files match\n", len(baseline.Files))
	return nil
}

// compareManifests treats the baseline as the authority. A file missing from
// the new render fails. A new file is an extra and does not fail unless the
// caller asks for a strict set comparison, because the catalog gate owns state
// coverage and the pixel baseline may be a stable subset.
func compareManifests(baseline, against manifest) (problems, extras []string) {
	base := map[string]fileRecord{}
	for _, f := range baseline.Files {
		base[f.Name] = f
	}
	next := map[string]fileRecord{}
	for _, f := range against.Files {
		next[f.Name] = f
	}
	for name, b := range base {
		a, ok := next[name]
		if !ok {
			problems = append(problems, "missing "+name)
			continue
		}
		if b.SHA256 != a.SHA256 {
			problems = append(problems, "changed "+name)
			continue
		}
		if b.Width != a.Width || b.Height != a.Height {
			problems = append(problems, fmt.Sprintf("size %s, baseline %dx%d against %dx%d", name, b.Width, b.Height, a.Width, a.Height))
		}
	}
	for name := range next {
		if _, ok := base[name]; !ok {
			extras = append(extras, name)
		}
	}
	sort.Strings(problems)
	sort.Strings(extras)
	return problems, extras
}

func readManifest(path string) (manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return manifest{}, err
	}
	var m manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return manifest{}, err
	}
	if len(m.Files) == 0 {
		return manifest{}, fmt.Errorf("%s lists no files", path)
	}
	return m, nil
}

func writeManifest(path string, m manifest) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func git(root string, args ...string) string {
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

func dirty(root string) string {
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	if strings.TrimSpace(string(out)) == "" {
		return "clean"
	}
	return "dirty"
}
