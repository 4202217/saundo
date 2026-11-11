// Command verify-map checks that the verification skill's feature map only
// asserts facts that still exist in the tree. Every TestXxx, SAUNDO_* gate,
// quoted locale string, .go path with a line citation, and Go symbol named in
// .claude/skills/verify/ must be findable in the source, so docs drift turns
// into a red gate instead of a wrong live verification.
//
// It replaces the per-item recursive greps of the old Bash checker with one
// pass over the tree: on Windows each grep spawn cost more than the scan.
//
// Classification of a backticked span:
//
//	CJK text          -> each CJK run must exist in a locale catalog or .go
//	bare identifier   -> must appear as a word in Go source; all-uppercase
//	                     wire vocabulary may live in protocol/devices docs
//	bare <file>.go(:n)-> file must exist, line must land inside it
//	anything else     -> commands, flags, placeholders: prose, skipped
package main

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const mapDir = ".claude/skills/verify"

var (
	testInDoc   = regexp.MustCompile(`Test[A-Z][A-Za-z0-9_]*`)
	envInDoc    = regexp.MustCompile(`SAUNDO_[A-Z0-9_]+`)
	goPathInDoc = regexp.MustCompile(`(cmd|internal|scripts)/[A-Za-z0-9_./-]+\.go(:[0-9]+(-[0-9]+)?)?`)
	backticks   = regexp.MustCompile("`[^`]+`")
	goCitation  = regexp.MustCompile(`^([A-Za-z0-9_./-]+/)?[A-Za-z0-9_]+\.go(:[0-9]+(-[0-9]+)?)?$`)
	cjkAny      = regexp.MustCompile(`[\x{4e00}-\x{9fff}]`)
	cjkRun      = regexp.MustCompile(`[\x{4e00}-\x{9fff}]{2,}`)
	symbolShape = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{3,}$`)
	uppercase   = regexp.MustCompile(`^[A-Z0-9_]+$`)
	// Spans that name things outside this tree on purpose: framework window
	// classes (pinned live by the doctor) and external tools.
	allowlist  = regexp.MustCompile(`^(MyGoFlyoutShadow|MyGoWindow|gofmt|taskkill|curl|powershell|mygo)$`)
	funcDecl   = regexp.MustCompile(`func\s+([A-Za-z_][A-Za-z0-9_]*)\(`)
	identifier = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*`)
	envGate    = regexp.MustCompile(`SAUNDO_[A-Z0-9_]+`)
)

// index is everything the map may assert, gathered in one pass.
type index struct {
	tests     map[string]bool
	envs      map[string]bool
	idents    map[string]bool
	goByBase  map[string]string // first sorted path for a bare .go citation
	lineCount map[string]int
	goText    string
	locale    string
	wire      string
}

func main() {
	root, err := findRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "verify-map: %v\n", err)
		os.Exit(2)
	}
	os.Exit(run(root))
}

// run performs the check and returns the process exit code. A missing or
// unreadable map directory is a setup error, not an empty map: the checker
// must fail closed when it cannot see what it is supposed to verify.
func run(root string) int {
	dir := filepath.Join(root, mapDir)
	fi, err := os.Stat(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "verify-map: map directory %s: %v\n", dir, err)
		return 2
	}
	if !fi.IsDir() {
		fmt.Fprintf(os.Stderr, "verify-map: map path %s is not a directory\n", dir)
		return 2
	}
	ix, err := loadIndex(root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "verify-map: %v\n", err)
		return 2
	}
	files, err := mapFiles(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "verify-map: %v\n", err)
		return 2
	}

	errors := 0
	fail := func(format string, args ...any) {
		fmt.Printf("FAIL "+format+"\n", args...)
		errors++
	}

	var spans []string
	for _, file := range files {
		base, err := filepath.Rel(dir, file)
		if err != nil {
			base = file
		}
		data, err := os.ReadFile(file)
		if err != nil {
			fail("%s: %v", base, err)
			continue
		}
		text := string(data)

		for _, name := range uniqueSorted(testInDoc.FindAllString(text, -1)) {
			if !ix.tests[name] {
				fail("%s: test %s does not exist in any _test.go", base, name)
			}
		}
		for _, ev := range uniqueSorted(envInDoc.FindAllString(text, -1)) {
			if !ix.envs[ev] {
				fail("%s: env gate %s not found in source", base, ev)
			}
		}
		for _, citation := range uniqueSorted(goPathInDoc.FindAllString(text, -1)) {
			path, lines := splitCitation(citation)
			if _, err := os.Stat(filepath.Join(root, path)); err != nil {
				fail("%s: path %s does not exist", base, path)
				continue
			}
			if lines != "" {
				total, err := countLines(root, ix, path)
				if err != nil {
					fail("%s: %v", base, err)
					continue
				}
				if n, ok := firstLine(lines); !ok || n > total {
					fail("%s: %s has %d lines; citation says %s", base, path, total, lines)
				}
			}
		}
		for _, line := range strings.Split(text, "\n") {
			for _, span := range backticks.FindAllString(line, -1) {
				spans = append(spans, span[1:len(span)-1])
			}
		}
	}

	for _, span := range uniqueSorted(spans) {
		if allowlist.MatchString(span) {
			continue
		}
		if goCitation.MatchString(span) {
			name, lines := splitBareCitation(span)
			path, ok := ix.goByBase[name]
			if !ok {
				fail("map: %s does not exist in the tree", name)
				continue
			}
			if lines != "" {
				total, err := countLines(root, ix, path)
				if err != nil {
					fail("map: %v", err)
					continue
				}
				if n, ok := firstLine(lines); !ok || n > total {
					fail("map: %s has %d lines; citation says %s", path, total, lines)
				}
			}
			continue
		}
		if cjkAny.MatchString(span) {
			for _, run := range cjkRun.FindAllString(span, -1) {
				if strings.Contains(ix.locale, run) || strings.Contains(ix.goText, run) {
					continue
				}
				fail("map: string %s not in locales or source", run)
			}
			continue
		}
		if symbolShape.MatchString(span) {
			if strings.HasPrefix(span, "Test") || strings.HasPrefix(span, "SAUNDO_") {
				continue
			}
			if ix.idents[span] {
				continue
			}
			if uppercase.MatchString(span) && strings.Contains(ix.wire, span) {
				continue
			}
			fail("map: symbol %s not found in source or docs", span)
		}
		// Everything else (commands, flags, placeholders, log lines) is prose.
	}

	if errors > 0 {
		fmt.Printf("%d stale map assertion(s)\n", errors)
		return 1
	}
	fmt.Println("map ok: every test name, env gate, path, string, and symbol resolves")
	return 0
}

// findRoot walks up from the working directory to the module root.
func findRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no go.mod above the working directory")
		}
		dir = parent
	}
}

// mapFiles lists SKILL.md and the feature pages.
func mapFiles(dir string) ([]string, error) {
	files := []string{filepath.Join(dir, "SKILL.md")}
	features, err := filepath.Glob(filepath.Join(dir, "features", "*.md"))
	if err != nil {
		return nil, err
	}
	sort.Strings(features)
	return append(files, features...), nil
}

// loadIndex reads the Go and locale sources the map may cite: content from
// cmd and internal, plus the .go names under scripts for bare citations.
func loadIndex(root string) (*index, error) {
	ix := &index{
		tests:     map[string]bool{},
		envs:      map[string]bool{},
		idents:    map[string]bool{},
		goByBase:  map[string]string{},
		lineCount: map[string]int{},
	}
	var goText strings.Builder
	var paths []string
	for _, dir := range []string{"cmd", "internal", "scripts"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(d.Name(), ".go") {
				return nil
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			paths = append(paths, rel)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(paths)
	for _, rel := range paths {
		if _, ok := ix.goByBase[filepath.Base(rel)]; !ok {
			ix.goByBase[filepath.Base(rel)] = rel
		}
	}
	for _, dir := range []string{"cmd", "internal"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(d.Name(), ".go") {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			goText.Write(data)
			goText.WriteByte('\n')
			if strings.HasSuffix(d.Name(), "_test.go") {
				for _, m := range funcDecl.FindAllSubmatch(data, -1) {
					ix.tests[string(m[1])] = true
				}
			}
			for _, m := range envGate.FindAll(data, -1) {
				ix.envs[string(m)] = true
			}
			for _, m := range identifier.FindAll(data, -1) {
				ix.idents[string(m)] = true
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	ix.goText = goText.String()

	locale, err := readDirText(filepath.Join(root, "cmd", "saundo", "locales"))
	if err != nil {
		return nil, err
	}
	ix.locale = locale
	wire, err := readFiles(filepath.Join(root, "docs", "protocol.md"), filepath.Join(root, "docs", "devices.md"))
	if err != nil {
		return nil, err
	}
	ix.wire = wire
	return ix, nil
}

// readDirText concatenates every file in dir, as the old recursive grep saw
// them.
func readDirText(dir string) (string, error) {
	var b strings.Builder
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		b.Write(data)
		b.WriteByte('\n')
		return nil
	})
	return b.String(), err
}

func readFiles(paths ...string) (string, error) {
	var b strings.Builder
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		b.Write(data)
		b.WriteByte('\n')
	}
	return b.String(), nil
}

// countLines counts a file's lines as wc -l does, cached per path.
func countLines(root string, ix *index, rel string) (int, error) {
	if n, ok := ix.lineCount[rel]; ok {
		return n, nil
	}
	data, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		return 0, err
	}
	n := bytes.Count(data, []byte("\n"))
	ix.lineCount[rel] = n
	return n, nil
}

// splitCitation separates a directory-prefixed .go citation from its line
// spec, e.g. "internal/app/x.go:12" -> ("internal/app/x.go", ":12").
func splitCitation(citation string) (path, lines string) {
	if i := strings.Index(citation, ".go:"); i >= 0 {
		return citation[:i+3], citation[i+3:]
	}
	return citation, ""
}

// splitBareCitation separates a bare .go citation from its line spec and
// returns the basename the tree is searched by.
func splitBareCitation(span string) (name, lines string) {
	if i := strings.LastIndex(span, ".go"); i >= 0 {
		name = filepath.Base(span[:i+3])
		lines = span[i+3:]
		return name, lines
	}
	return filepath.Base(span), ""
}

// firstLine reads the first number of a ":12" or ":12-20" spec.
func firstLine(spec string) (int, bool) {
	spec = strings.TrimPrefix(spec, ":")
	if i := strings.IndexByte(spec, '-'); i >= 0 {
		spec = spec[:i]
	}
	n, err := strconv.Atoi(spec)
	return n, err == nil
}

func uniqueSorted(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	sort.Strings(values)
	out := values[:1]
	for _, v := range values[1:] {
		if v != out[len(out)-1] {
			out = append(out, v)
		}
	}
	return out
}
